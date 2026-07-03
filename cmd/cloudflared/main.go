package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/automaxprocs/maxprocs"

	"github.com/cloudflare/cloudflared/metrics"
)

type appConfig struct {
	ListenAddr     string
	QuickService   string
	EdgeIPVersion  string
	Protocol       string
	MaxTunnels     int
	TunnelTimeout  time.Duration
	LogLevel       string
	AllowedOrigins string
}

func loadConfig() appConfig {
	return appConfig{
		ListenAddr:     env("LISTEN_ADDR", ":8787"),
		QuickService:   env("QUICK_SERVICE_URL", "https://api.trycloudflare.com"),
		EdgeIPVersion:  env("EDGE_IP_VERSION", "4"),
		Protocol:       env("PROTOCOL", "quic"),
		MaxTunnels:     envInt("MAX_TUNNELS", 100),
		TunnelTimeout:  envDur("TUNNEL_TIMEOUT", 30*time.Second),
		LogLevel:       env("LOG_LEVEL", "info"),
		AllowedOrigins: env("ALLOWED_ORIGINS", "*"),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}

var (
	Version   = "DEV"
	BuildTime = "unknown"
	BuildType = ""

	serverCtx    context.Context
	serverCancel context.CancelFunc

	runningMu sync.Mutex
	running   = map[string]*tunnelHandle{}
)

type quickTunnelRequest struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"`
	Proxy    string `json:"proxy,omitempty"`
}

func (r quickTunnelRequest) validate() error {
	if r.Host == "" {
		return errors.New("host is required")
	}
	if r.Port <= 0 || r.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	p := strings.ToLower(r.Protocol)
	if p != "" && p != "quic" && p != "http2" {
		return errors.New("protocol must be 'quic' or 'http2'")
	}
	if r.Proxy != "" {
		if !strings.HasPrefix(r.Proxy, "http://") &&
			!strings.HasPrefix(r.Proxy, "https://") &&
			!strings.HasPrefix(r.Proxy, "socks5://") &&
			!strings.HasPrefix(r.Proxy, "socks5h://") {
			return errors.New("proxy must start with http://, https://, socks5://, or socks5h://")
		}
	}
	return nil
}

type tunnelStatus string

const (
	statusStarting   tunnelStatus = "starting"
	statusStarted    tunnelStatus = "started"
	statusStopped    tunnelStatus = "stopped"
	statusError      tunnelStatus = "error"
	statusNotFound   tunnelStatus = "not_found"
	statusTimeout    tunnelStatus = "timeout"
	statusLimitReach tunnelStatus = "limit_reached"
)

type tunnelHandle struct {
	id       string
	target   string
	proxy    string
	protocol string

	status   tunnelStatus
	url      string
	err      error
	pid      int
	startedAt time.Time

	cmd  *exec.Cmd
	done chan struct{}

	mu sync.Mutex
}

func (h *tunnelHandle) snapshot() tunnelSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()

	s := tunnelSnapshot{
		ID:        h.id,
		Target:    h.target,
		Protocol:  h.protocol,
		Proxy:     h.proxy,
		Status:    h.status,
		PID:       h.pid,
		StartedAt: h.startedAt,
	}
	if h.url != "" {
		s.URL = h.url
	}
	if h.err != nil {
		s.Error = h.err.Error()
	}
	return s
}

type tunnelSnapshot struct {
	ID        string       `json:"id"`
	URL       string       `json:"url,omitempty"`
	Target    string       `json:"target"`
	Protocol  string       `json:"protocol"`
	Proxy     string       `json:"proxy,omitempty"`
	Status    tunnelStatus `json:"status"`
	Error     string       `json:"error,omitempty"`
	PID       int          `json:"pid,omitempty"`
	StartedAt time.Time    `json:"started_at"`
}

type quickURLHub struct {
	mu sync.Mutex
	ch map[string]chan string
}

func newQuickURLHub() *quickURLHub {
	return &quickURLHub{ch: make(map[string]chan string)}
}

func (h *quickURLHub) register(id string) chan string {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := make(chan string, 1)
	h.ch[id] = c
	return c
}

func (h *quickURLHub) unregister(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.ch, id)
}

func (h *quickURLHub) push(id, url string) {
	h.mu.Lock()
	c := h.ch[id]
	h.mu.Unlock()
	if c != nil {
		select {
		case c <- url:
		default:
		}
	}
}

var hub = newQuickURLHub()

var zl zerolog.Logger

func initLogger(cfg appConfig) {
	lvl, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	output := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
		NoColor:    false,
	}
	zl = zerolog.New(output).Level(lvl).With().Timestamp().Logger()
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, req)
		zl.Debug().
			Str("method", req.Method).
			Str("path", req.URL.Path).
			Int("status", rec.status).
			Dur("dur", time.Since(start)).
			Str("remote", req.RemoteAddr).
			Msg("request")
	})
}

func withCORS(origins string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if origins == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origins != "" {
				origin := req.Header.Get("Origin")
				for _, o := range strings.Split(origins, ",") {
					if strings.TrimSpace(o) == origin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						break
					}
				}
			}

			if req.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		zl.Err(err).Msg("failed to encode JSON response")
	}
}

func writeJSONStatus(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		zl.Err(err).Msg("failed to encode JSON error response")
	}
}

type apiError struct {
	Error string `json:"error"`
}

func sendError(w http.ResponseWriter, code int, msg string) {
	writeJSONStatus(w, code, apiError{Error: msg})
}

func postJSON(url string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

func randID(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type server struct {
	cfg    appConfig
	cfgMu  sync.RWMutex
}

func newServer(cfg appConfig) *server {
	return &server{cfg: cfg}
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *server) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"service": "shitflared",
		"version": Version,
		"docs":    "see README.md for API documentation",
	})
}

func (s *server) handleListTunnels(w http.ResponseWriter, r *http.Request) {
	runningMu.Lock()
	out := make([]tunnelSnapshot, 0, len(running))
	for _, h := range running {
		out = append(out, h.snapshot())
	}
	runningMu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.Before(out[j].StartedAt)
	})

	writeJSON(w, out)
}

func (s *server) handleStartQuickTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendError(w, http.StatusBadRequest, "cannot read body")
		return
	}

	var req quickTunnelRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := req.validate(); err != nil {
		sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.cfgMu.RLock()
	cfg := s.cfg
	s.cfgMu.RUnlock()

	runningMu.Lock()
	tunnelCount := len(running)
	runningMu.Unlock()

	if tunnelCount >= cfg.MaxTunnels {
		sendError(w, http.StatusServiceUnavailable,
			fmt.Sprintf("max tunnels reached (%d)", cfg.MaxTunnels))
		return
	}

	id := randID(12)
	target := fmt.Sprintf("%s:%d", req.Host, req.Port)

	protocol := strings.ToLower(req.Protocol)
	if protocol == "" {
		protocol = cfg.Protocol
	}

	if req.Proxy != "" && protocol == "quic" {
		protocol = "http2"
		zl.Debug().Str("tunnel_id", id).Msg("proxy set – switching to http2 protocol")
	}

	urlCh := hub.register(id)
	defer hub.unregister(id)

	exe, err := os.Executable()
	if err != nil {
		sendError(w, http.StatusInternalServerError, "cannot determine executable path")
		return
	}

	cbURL := fmt.Sprintf("http://127.0.0.1%s/internal/quick-url", cbListenPort(cfg.ListenAddr))

	cmd := exec.Command(exe,
		"--worker",
		"--id", id,
		"--target", target,
		"--protocol", protocol,
		"--callback", cbURL,
		"--rpc-timeout", "30s",
	)

	cmd.Env = append(os.Environ(),
		"QUICK_REQUEST_ID="+id,
	)
	if req.Proxy != "" {
		cmd.Env = append(cmd.Env, "ALL_PROXY="+req.Proxy, "NO_PROXY=127.0.0.1,localhost,::1")
		if !strings.HasPrefix(req.Proxy, "socks5") {
			cmd.Env = append(cmd.Env,
				"HTTP_PROXY="+req.Proxy,
				"HTTPS_PROXY="+req.Proxy,
			)
		}
	}

	h := &tunnelHandle{
		id:        id,
		target:    target,
		proxy:     req.Proxy,
		protocol:  protocol,
		status:    statusStarting,
		cmd:       cmd,
		done:      make(chan struct{}),
		startedAt: time.Now(),
	}

	runningMu.Lock()
	running[id] = h
	runningMu.Unlock()

	if err := cmd.Start(); err != nil {
		runningMu.Lock()
		delete(running, id)
		runningMu.Unlock()
		zl.Err(err).Str("tunnel_id", id).Str("target", target).Msg("tunnel start failed")
		sendError(w, http.StatusInternalServerError, "failed to start tunnel: "+err.Error())
		return
	}

	h.mu.Lock()
	h.pid = cmd.Process.Pid
	h.mu.Unlock()
	zl.Info().Str("tunnel_id", id).Str("target", target).Int("pid", cmd.Process.Pid).Msg("tunnel process started")

	go func() {
		err := cmd.Wait()
		h.mu.Lock()
		if err != nil {
			h.err = err
			if h.status != statusTimeout && h.status != statusStopped {
				h.status = statusError
			}
		} else {
			h.status = statusStopped
		}
		h.mu.Unlock()
		close(h.done)
	}()

	select {
	case url := <-urlCh:
		h.mu.Lock()
		h.url = url
		h.status = statusStarted
		h.mu.Unlock()
		zl.Info().Str("tunnel_id", id).Str("url", url).Str("target", target).Int("pid", cmd.Process.Pid).Msg("tunnel ready")
		writeJSON(w, tunnelSnapshot{
			ID:        id,
			URL:       url,
			Target:    target,
			Protocol:  protocol,
			Proxy:     req.Proxy,
			Status:    statusStarted,
			PID:       cmd.Process.Pid,
			StartedAt: h.startedAt,
		})

	case <-h.done:
		snap := h.snapshot()
		runningMu.Lock()
		delete(running, id)
		runningMu.Unlock()
		code := http.StatusInternalServerError
		if snap.Error == "" {
			code = http.StatusOK
			snap.Status = statusStopped
			zl.Warn().Str("tunnel_id", id).Str("target", target).Msg("tunnel stopped before getting URL")
		} else {
			zl.Err(fmt.Errorf("%s", snap.Error)).Str("tunnel_id", id).Msg("tunnel error")
		}
		writeJSONStatus(w, code, snap)

	case <-time.After(cfg.TunnelTimeout):
		h.mu.Lock()
		h.status = statusTimeout
		h.mu.Unlock()
		zl.Warn().Str("tunnel_id", id).Dur("timeout", cfg.TunnelTimeout).Msg("tunnel timed out")
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		runningMu.Lock()
		delete(running, id)
		runningMu.Unlock()
		sendError(w, http.StatusGatewayTimeout, fmt.Sprintf(
			"timed out waiting for quick tunnel URL (%v)", cfg.TunnelTimeout))
	}
}

func (s *server) handleStopTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendError(w, http.StatusBadRequest, "cannot read body")
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		sendError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.ID == "" {
		sendError(w, http.StatusBadRequest, "missing 'id' field")
		return
	}

	runningMu.Lock()
	h := running[req.ID]
	runningMu.Unlock()

	if h == nil {
		sendError(w, http.StatusNotFound, fmt.Sprintf("tunnel %s not found", req.ID))
		return
	}

	snap := h.snapshot()
	zl.Info().Str("tunnel_id", snap.ID).Str("url", snap.URL).Str("target", snap.Target).Msg("stopping tunnel")

	h.mu.Lock()
	h.status = statusStopped
	h.mu.Unlock()

	if h.cmd != nil && h.cmd.Process != nil {
		if err := h.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			_ = h.cmd.Process.Kill()
		}
	}

	select {
	case <-h.done:
		zl.Debug().Str("tunnel_id", snap.ID).Msg("tunnel process exited")
	case <-time.After(5 * time.Second):
		zl.Warn().Str("tunnel_id", snap.ID).Msg("tunnel stop timeout, forcing")
	}

	runningMu.Lock()
	delete(running, req.ID)
	runningMu.Unlock()

	snap.Status = statusStopped
	writeJSON(w, snap)
}

func (s *server) handleWorkerCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendError(w, http.StatusBadRequest, "cannot read body")
		return
	}

	var cb struct {
		ID     string `json:"id"`
		URL    string `json:"url"`
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &cb); err != nil {
		sendError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if cb.ID == "" {
		sendError(w, http.StatusBadRequest, "missing 'id' field")
		return
	}

	runningMu.Lock()
	h := running[cb.ID]
	runningMu.Unlock()

	if h != nil {
		h.mu.Lock()
		if cb.Status == "error" && cb.Error != "" {
			h.err = fmt.Errorf("%s", cb.Error)
			h.status = statusError
			zl.Err(h.err).Str("tunnel_id", cb.ID).Msg("worker reported error")
		}
		if cb.URL != "" {
			h.url = cb.URL
			hub.push(cb.ID, cb.URL)
			zl.Debug().Str("tunnel_id", cb.ID).Str("url", cb.URL).Msg("worker reported URL")
		}
		h.mu.Unlock()
	}

	w.WriteHeader(http.StatusNoContent)
}

func cbListenPort(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ":8787"
	}
	return ":" + port
}

func main() {
	if isWorkerMode() {
		os.Exit(workerMain())
	}

	cfg := loadConfig()
	initLogger(cfg)

	os.Setenv("QUIC_GO_DISABLE_ECN", "1")
	metrics.RegisterBuildInfo(BuildType, BuildTime, Version)
	_, _ = maxprocs.Set()

	zl.Info().
		Str("listen", cfg.ListenAddr).
		Str("quick_service", cfg.QuickService).
		Str("protocol", cfg.Protocol).
		Int("max_tunnels", cfg.MaxTunnels).
		Dur("tunnel_timeout", cfg.TunnelTimeout).
		Msg("shitflared starting")

	serverCtx, serverCancel = context.WithCancel(context.Background())
	defer serverCancel()

	srv := newServer(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", srv.handleHealth)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			srv.handleRoot(w, r)
			return
		}
		switch r.URL.Path {
		case "/tunnel/quick":
			srv.handleStartQuickTunnel(w, r)
		case "/tunnel/stop":
			srv.handleStopTunnel(w, r)
		case "/tunnel/list":
			srv.handleListTunnels(w, r)
		case "/internal/quick-url":
			srv.handleWorkerCallback(w, r)
		default:
			sendError(w, http.StatusNotFound, "not found")
		}
	})

	handler := withLogging(withCORS(cfg.AllowedOrigins)(mux))

	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		zl.Info().Str("signal", sig.String()).Msg("shutting down")
		serverCancel()

		runningMu.Lock()
		n := len(running)
		if n > 0 {
			zl.Info().Int("count", n).Msg("stopping active tunnels")
		}
		for id, h := range running {
			if h.cmd != nil && h.cmd.Process != nil {
				_ = h.cmd.Process.Signal(syscall.SIGTERM)
			}
			h.mu.Lock()
			h.status = statusStopped
			h.mu.Unlock()
			zl.Debug().Str("tunnel_id", id).Msg("stopping tunnel")
		}
		runningMu.Unlock()

		done := make(chan struct{})
		go func() {
			runningMu.Lock()
			for _, h := range running {
				select {
				case <-h.done:
				case <-time.After(3 * time.Second):
				}
			}
			runningMu.Unlock()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(10 * time.Second):
			zl.Warn().Msg("forced shutdown after timeout")
		}

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			zl.Err(err).Msg("http server shutdown error")
		}
	}()

	zl.Info().Str("addr", cfg.ListenAddr).Msg("listening")
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		zl.Fatal().Err(err).Msg("server error")
	}
	zl.Info().Msg("server stopped")
}

func init() {
	logWriter := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
		NoColor:    false,
	}
	zl := zerolog.New(logWriter).Level(zerolog.WarnLevel).With().Timestamp().Logger()
	stdlog.SetFlags(0)
	stdlog.SetOutput(zl)
}
