package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	bk8s "github.com/jlaska/bbroker/internal/k8s"
	"k8s.io/client-go/kubernetes"
	"nhooyr.io/websocket"
)

// SessionInfo tracks a live session for /status reporting.
type SessionInfo struct {
	ID      string    `json:"id"`
	Browser string    `json:"browser"`
	PodIP   string    `json:"podIP"`
	Started time.Time `json:"started"`
}

// SessionManager creates and destroys browser pods per client connection.
type SessionManager struct {
	client       kubernetes.Interface
	namespace    string
	browserImage string
	browserArgs  []string
	wardenImage  string
	xvfbImage    string
	directWSPort int // non-zero = skip CDP discovery, connect directly to this port

	mu       sync.Mutex
	sessions map[string]*SessionInfo
}

func NewSessionManager(client kubernetes.Interface, cfg Config) *SessionManager {
	return &SessionManager{
		client:       client,
		namespace:    cfg.Namespace,
		browserImage: cfg.BrowserImage,
		browserArgs:  cfg.BrowserArgs,
		wardenImage:  cfg.WardenImage,
		xvfbImage:    cfg.XvfbImage,
		directWSPort: cfg.DirectWSPort,
		sessions:     make(map[string]*SessionInfo),
	}
}

// Handle manages the full lifecycle of one browser session.
func (sm *SessionManager) Handle(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	params := r.URL.Query()
	headful := params.Get("headful") == "true"
	browser := "chrome"

	sessionID := newSessionID()
	log := slog.With("session", sessionID)
	log.Info("new session request", "headful", headful, "remoteAddr", r.RemoteAddr)

	// Upgrade the client WebSocket before creating the pod so the client
	// stays connected while the pod starts.
	clientConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Error("ws accept", "err", err)
		podCreationErrors.WithLabelValues("ws_accept").Inc()
		return
	}
	defer clientConn.CloseNow()

	sessionsTotal.WithLabelValues(browser).Inc()
	activeSessions.WithLabelValues(browser).Inc()
	defer activeSessions.WithLabelValues(browser).Dec()

	cfg := bk8s.SessionConfig{
		SessionID:    sessionID,
		Namespace:    sm.namespace,
		BrowserImage: sm.browserImage,
		BrowserArgs:  sm.browserArgs,
		WardenImage:  sm.wardenImage,
		Headful:      headful,
		XvfbImage:    sm.xvfbImage,
		Params:       params,
	}

	pod, err := bk8s.CreateBrowserPod(ctx, sm.client, cfg)
	if err != nil {
		log.Error("create pod", "err", err)
		podCreationErrors.WithLabelValues("create").Inc()
		clientConn.Close(websocket.StatusInternalError, "could not create browser pod")
		return
	}
	defer func() {
		delCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := bk8s.DeletePod(delCtx, sm.client, sm.namespace, pod.Name); err != nil {
			log.Warn("delete pod", "err", err)
		}
	}()

	log.Info("pod created, waiting for ready", "pod", pod.Name)
	podIP, err := bk8s.WaitForPodReady(ctx, sm.client, sm.namespace, pod.Name)
	if err != nil {
		log.Error("wait for pod", "err", err)
		podCreationErrors.WithLabelValues("wait_ready").Inc()
		clientConn.Close(websocket.StatusInternalError, "browser pod failed to start")
		return
	}

	var cdpURL string
	if sm.directWSPort > 0 {
		// Proxy-image mode (e.g. sockpuppetbrowser): connect directly to the
		// image's WebSocket port, forwarding client query params so flags like
		// ?headful=true reach the browser image.
		cdpURL = fmt.Sprintf("ws://%s:%d/?%s", podIP, sm.directWSPort, params.Encode())
		log.Info("pod ready, using direct WebSocket", "podIP", podIP, "port", sm.directWSPort)
	} else {
		log.Info("pod ready, discovering CDP endpoint", "podIP", podIP)
		var err error
		cdpURL, err = DiscoverCDPEndpoint(podIP, bk8s.CDPPort)
		if err != nil {
			log.Error("discover CDP", "err", err)
			podCreationErrors.WithLabelValues("cdp_discover").Inc()
			clientConn.Close(websocket.StatusInternalError, "browser CDP not ready")
			return
		}
		cdpURL = rewriteHost(cdpURL, podIP)
	}
	log.Info("relaying CDP", "cdpURL", cdpURL, "startupLatency", time.Since(start).Round(time.Millisecond))
	sessionStartupLatency.Observe(time.Since(start).Seconds())

	chromeConn, _, err := websocket.Dial(ctx, cdpURL, nil)
	if err != nil {
		log.Error("dial chrome", "err", err)
		podCreationErrors.WithLabelValues("dial_chrome").Inc()
		clientConn.Close(websocket.StatusInternalError, "could not connect to browser")
		return
	}
	defer chromeConn.CloseNow()

	info := &SessionInfo{ID: sessionID, Browser: browser, PodIP: podIP, Started: start}
	sm.addSession(info)
	defer sm.removeSession(sessionID)
	defer func() { sessionDuration.Observe(time.Since(start).Seconds()) }()

	log.Info("CDP relay established")
	relay(ctx, clientConn, chromeConn)
	log.Info("session closed", "duration", time.Since(start).Round(time.Second))
}

func relay(ctx context.Context, a, b *websocket.Conn) {
	errCh := make(chan error, 2)
	pipe := func(src, dst *websocket.Conn, dir string) {
		for {
			mt, data, err := src.Read(ctx)
			if err != nil {
				errCh <- err
				return
			}
			cdpFrames.WithLabelValues(dir).Inc()
			if err := dst.Write(ctx, mt, data); err != nil {
				errCh <- err
				return
			}
		}
	}
	go pipe(a, b, "in")
	go pipe(b, a, "out")
	<-errCh
}

func (sm *SessionManager) addSession(info *SessionInfo) {
	sm.mu.Lock()
	sm.sessions[info.ID] = info
	sm.mu.Unlock()
}

func (sm *SessionManager) removeSession(id string) {
	sm.mu.Lock()
	delete(sm.sessions, id)
	sm.mu.Unlock()
}

// Sessions returns a snapshot of current sessions for /status.
func (sm *SessionManager) Sessions() []SessionInfo {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	out := make([]SessionInfo, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		out = append(out, *s)
	}
	return out
}

func rewriteHost(cdpURL, podIP string) string {
	u, err := url.Parse(cdpURL)
	if err != nil {
		return cdpURL
	}
	u.Host = fmt.Sprintf("%s:%s", podIP, u.Port())
	if u.Port() == "" {
		u.Host = fmt.Sprintf("%s:%d", podIP, bk8s.CDPPort)
	}
	return u.String()
}

func newSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("session-%s", hex.EncodeToString(b))
}
