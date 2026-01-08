package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/automaxprocs/maxprocs"

	"github.com/cloudflare/cloudflared/metrics"
)

var (
	Version   = "DEV"
	BuildTime = "unknown"
	BuildType = ""
)

type quickTunnelRequest struct {
	Host  string `json:"host"`
	Port  int    `json:"port"`
	Proxy string `json:"proxy,omitempty"`
}

type quickTunnelResponse struct {
	ID       string `json:"id"`
	URL      string `json:"url,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	PID      int    `json:"pid,omitempty"`
	Target   string `json:"target,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Proxy    string `json:"proxy,omitempty"`
}

type workerCallback struct {
	ID     string `json:"id"`
	URL    string `json:"url,omitempty"`
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

type tunnelHandle struct {
	id     string
	url    string
	target string
	proxy  string

	protocol string
	cmd      *exec.Cmd
	done     chan struct{}

	mu  sync.Mutex
	err error
}

type quickURLHub struct {
	mu sync.Mutex
	ch map[string]chan string
}

func newQuickURLHub() *quickURLHub { return &quickURLHub{ch: make(map[string]chan string)} }

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

var (
	serverCtx    context.Context
	serverCancel context.CancelFunc

	hub = newQuickURLHub()

	runningMu sync.Mutex
	running   = map[string]*tunnelHandle{}
)

func main() {
	if isWorkerMode() {
		os.Exit(workerMain())
	}

	os.Setenv("QUIC_GO_DISABLE_ECN", "1")
	metrics.RegisterBuildInfo(BuildType, BuildTime, Version)
	_, _ = maxprocs.Set()

	serverCtx, serverCancel = context.WithCancel(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel/quick", handleStartQuickTunnel)
	mux.HandleFunc("/tunnel/stop", handleStopTunnel)
	mux.HandleFunc("/tunnel/list", handleListTunnels)

	mux.HandleFunc("/internal/quick-url", handleWorkerCallback)

	srv := &http.Server{Addr: ":8787", Handler: mux}
	go func() { _ = srv.ListenAndServe() }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	serverCancel()

	runningMu.Lock()
	for _, h := range running {
		if h != nil && h.cmd != nil && h.cmd.Process != nil {
			_ = h.cmd.Process.Kill()
		}
	}
	runningMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func handleStartQuickTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req quickTunnelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Host == "" || req.Port <= 0 || req.Port > 65535 {
		http.Error(w, "invalid host/port", http.StatusBadRequest)
		return
	}

	id := randID(12)
	target := fmt.Sprintf("%s:%d", req.Host, req.Port)

	protocol := "quic"
	if req.Proxy != "" {
		protocol = "http2"
	}

	urlCh := hub.register(id)
	defer hub.unregister(id)

	exe, err := os.Executable()
	if err != nil {
		writeJSONStatus(w, 500, quickTunnelResponse{ID: id, Status: "error", Error: err.Error()})
		return
	}

	cbURL := "http://127.0.0.1:8787/internal/quick-url"

	cmd := exec.Command(exe,
		"--worker",
		"--id", id,
		"--target", target,
		"--protocol", protocol,
		"--callback", cbURL,
	)

	cmd.Env = append(os.Environ(),
		"QUICK_REQUEST_ID="+id,
	)
	if req.Proxy != "" {
		cmd.Env = append(cmd.Env,
			"HTTP_PROXY="+req.Proxy,
			"HTTPS_PROXY="+req.Proxy,
			"ALL_PROXY="+req.Proxy,
			"NO_PROXY=127.0.0.1,localhost",
		)
	}

	h := &tunnelHandle{
		id:       id,
		target:   target,
		proxy:    req.Proxy,
		protocol: protocol,
		cmd:      cmd,
		done:     make(chan struct{}),
	}

	runningMu.Lock()
	running[id] = h
	runningMu.Unlock()

	if err := cmd.Start(); err != nil {
		runningMu.Lock()
		delete(running, id)
		runningMu.Unlock()

		writeJSONStatus(w, 500, quickTunnelResponse{ID: id, Status: "error", Error: err.Error()})
		return
	}

	go func() {
		defer close(h.done)
		err := cmd.Wait()
		h.mu.Lock()
		h.err = err
		h.mu.Unlock()
	}()

	select {
	case url := <-urlCh:
		h.mu.Lock()
		h.url = url
		h.mu.Unlock()
		writeJSON(w, quickTunnelResponse{
			ID:       id,
			URL:      url,
			Status:   "started",
			PID:      cmd.Process.Pid,
			Target:   target,
			Protocol: protocol,
			Proxy:    req.Proxy,
		})
		return

	case <-h.done:
		h.mu.Lock()
		e := h.err
		h.mu.Unlock()

		runningMu.Lock()
		delete(running, id)
		runningMu.Unlock()

		if e == nil {
			writeJSON(w, quickTunnelResponse{ID: id, Status: "stopped"})
			return
		}
		writeJSONStatus(w, 500, quickTunnelResponse{ID: id, Status: "error", Error: e.Error()})
		return

	case <-time.After(30 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		writeJSONStatus(w, http.StatusGatewayTimeout, quickTunnelResponse{
			ID:     id,
			Status: "timeout",
			Error:  "timed out waiting for quick tunnel url",
		})
		return
	}
}

func handleStopTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	runningMu.Lock()
	h := running[body.ID]
	runningMu.Unlock()

	if h == nil {
		writeJSONStatus(w, http.StatusNotFound, quickTunnelResponse{ID: body.ID, Status: "not_found"})
		return
	}

	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
	}

	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
	}

	runningMu.Lock()
	delete(running, body.ID)
	runningMu.Unlock()

	writeJSON(w, quickTunnelResponse{ID: body.ID, Status: "stopped"})
}

func handleListTunnels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	type item struct {
		ID       string `json:"id"`
		URL      string `json:"url,omitempty"`
		Status   string `json:"status"`
		Error    string `json:"error,omitempty"`
		PID      int    `json:"pid,omitempty"`
		Target   string `json:"target,omitempty"`
		Protocol string `json:"protocol,omitempty"`
		Proxy    string `json:"proxy,omitempty"`
	}

	runningMu.Lock()
	out := make([]item, 0, len(running))
	for id, h := range running {
		h.mu.Lock()
		url := h.url
		err := h.err
		h.mu.Unlock()

		st := "running"
		e := ""
		if err != nil {
			st = "error"
			e = err.Error()
		}

		pid := 0
		if h.cmd != nil && h.cmd.Process != nil {
			pid = h.cmd.Process.Pid
		}

		out = append(out, item{
			ID:       id,
			URL:      url,
			Status:   st,
			Error:    e,
			PID:      pid,
			Target:   h.target,
			Protocol: h.protocol,
			Proxy:    h.proxy,
		})
	}
	runningMu.Unlock()

	writeJSON(w, out)
}

func handleWorkerCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var cb workerCallback
	if err := json.NewDecoder(r.Body).Decode(&cb); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cb.ID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	runningMu.Lock()
	h := running[cb.ID]
	runningMu.Unlock()

	if h != nil {
		h.mu.Lock()
		if cb.Status == "error" && cb.Error != "" {
			h.err = fmt.Errorf("%s", cb.Error)
		}
		if cb.URL != "" {
			h.url = cb.URL
			hub.push(cb.ID, cb.URL)
		}
		h.mu.Unlock()
	}

	w.WriteHeader(http.StatusNoContent)
}

func randID(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONStatus(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func postJSON(url string, v any) error {
	b, _ := json.Marshal(v)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
