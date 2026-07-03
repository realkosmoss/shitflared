package main

import (
	"context"
	"os"
	"time"

	"github.com/urfave/cli/v2"
	"go.uber.org/automaxprocs/maxprocs"

	"github.com/cloudflare/cloudflared/cmd/cloudflared/cliutil"
	"github.com/cloudflare/cloudflared/cmd/cloudflared/tunnel"
	"github.com/cloudflare/cloudflared/metrics"
	"github.com/cloudflare/cloudflared/token"
	"github.com/cloudflare/cloudflared/tracing"
)

type workerCallback struct {
	ID     string `json:"id"`
	URL    string `json:"url,omitempty"`
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

func isWorkerMode() bool {
	for _, a := range os.Args[1:] {
		if a == "--worker" {
			return true
		}
	}
	return false
}

func arg(flag string) string {
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == flag && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

func argOrEnv(flag, envKey string) string {
	if v := arg(flag); v != "" {
		return v
	}
	return os.Getenv(envKey)
}

type httpQuickSink struct {
	callback string
}

func (s *httpQuickSink) OnQuickTunnelURL(requestID, url string) {
	_ = postJSON(s.callback, workerCallback{
		ID:     requestID,
		URL:    url,
		Status: "url",
	})
}

func workerMain() int {
	id := argOrEnv("--id", "QUICK_REQUEST_ID")
	target := argOrEnv("--target", "TUNNEL_TARGET")
	protocol := argOrEnv("--protocol", "TUNNEL_PROTOCOL")
	callback := argOrEnv("--callback", "TUNNEL_CALLBACK")
	edgeIPVersion := argOrEnv("--edge-ip-version", "EDGE_IP_VERSION")
	quickService := argOrEnv("--quick-service", "QUICK_SERVICE_URL")

	if id == "" {
		id = "unknown"
	}
	if protocol == "" {
		protocol = "quic"
	}
	if target == "" {
		target = "127.0.0.1:8080"
	}
	if edgeIPVersion == "" {
		edgeIPVersion = "4"
	}
	if quickService == "" {
		quickService = "https://api.trycloudflare.com"
	}

	os.Setenv("QUIC_GO_DISABLE_ECN", "1")
	metrics.RegisterBuildInfo(BuildType, BuildTime, Version)
	_, _ = maxprocs.Set()

	buildInfo := cliutil.GetBuildInfo(BuildType, Version)
	graceShutdownC := make(chan struct{})

	tracing.Init(Version)
	token.Init(Version)
	tunnel.Init(buildInfo, graceShutdownC)

	if callback != "" {
		tunnel.SetQuickURLSink(&httpQuickSink{callback: callback})
	}

	app := &cli.App{
		Name:     "cloudflared",
		Version:  Version,
		Commands: tunnel.Commands(),
	}

	ctx := context.WithValue(context.Background(), "quick_request_id", id)

	args := []string{
		"cloudflared",
		"tunnel",
		"--url", target,
		"--metrics", "127.0.0.1:0",
		"--protocol", protocol,
		"--edge-ip-version", edgeIPVersion,
		"--quick-service", quickService,
		"--no-autoupdate",
	}

	err := app.RunContext(ctx, args)
	if err != nil && callback != "" {
		_ = postJSON(callback, workerCallback{
			ID:     id,
			Status: "error",
			Error:  err.Error(),
		})
		time.Sleep(150 * time.Millisecond)
		return 1
	}

	return 0
}
