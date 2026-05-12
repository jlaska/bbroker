package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Config holds proxy server configuration.
type Config struct {
	ListenAddr   string
	MetricsAddr  string
	Namespace    string
	BrowserImage string
	// BrowserArgs overrides the browser container entrypoint args.
	// Leave nil to use the image's built-in entrypoint (e.g. chromedp/headless-shell).
	BrowserArgs []string
	WardenImage string
	XvfbImage   string
	// DirectWSPort, when non-zero, causes bbrokerd to skip CDP /json/version
	// discovery and connect directly to ws://pod_ip:{DirectWSPort}.
	// Query params from the client URL (e.g. ?headful=true) are forwarded.
	// Use this for "proxy-image" browsers like sockpuppetbrowser that expose
	// a WebSocket endpoint directly rather than Chrome's raw CDP port.
	DirectWSPort int
	// HeadfulBrowserImage, when set, is used for ?headful=true sessions.
	// bbrokerd generates explicit Chrome CLI args (BuildChromeArgs) for this
	// image. Use a full Chromium image (e.g. bbroker-chrome) that accepts
	// args and can connect to the Xvfb sidecar via DISPLAY=:99.
	HeadfulBrowserImage    string
	BrowserImagePullPolicy string // e.g. "IfNotPresent", "Always"; empty = IfNotPresent
}

// Server is the bbrokerd HTTP/WebSocket server.
type Server struct {
	cfg     Config
	manager *SessionManager
}

func NewServer(cfg Config, manager *SessionManager) *Server {
	return &Server{cfg: cfg, manager: manager}
}

func (s *Server) Run(ctx context.Context) error {
	// Metrics on a separate port.
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsSrv := &http.Server{Addr: s.cfg.MetricsAddr, Handler: metricsMux}
	go func() {
		slog.Info("metrics listening", "addr", s.cfg.MetricsAddr)
		if err := metricsSrv.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("metrics server", "err", err)
		}
	}()

	// Main API server.
	mux := http.NewServeMux()
	mux.HandleFunc("/cdtp/chrome", s.handleCDTP)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/health", s.handleHealth)

	srv := &http.Server{
		Addr:    s.cfg.ListenAddr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		metricsSrv.Shutdown(shutCtx)
		srv.Shutdown(shutCtx)
	}()

	slog.Info("bbrokerd listening", "addr", s.cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleCDTP(w http.ResponseWriter, r *http.Request) {
	s.manager.Handle(r.Context(), w, r)
}

type statusResponse struct {
	ActiveSessions int           `json:"activeSessions"`
	Sessions       []SessionInfo `json:"sessions"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	sessions := s.manager.Sessions()
	resp := statusResponse{
		ActiveSessions: len(sessions),
		Sessions:       sessions,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("encode status", "err", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, `{"status":"ok"}`)
}
