package proxy

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	activeSessions = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "bbroker_active_sessions",
		Help: "Number of currently active browser sessions.",
	}, []string{"browser"})

	sessionStartupLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "bbroker_session_startup_latency_seconds",
		Help:    "Time from client connect to CDP relay established.",
		Buckets: []float64{0.5, 1, 2, 3, 5, 10, 20, 30},
	})

	sessionDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "bbroker_session_duration_seconds",
		Help:    "Duration of completed browser sessions.",
		Buckets: []float64{5, 30, 60, 300, 600, 1800, 3600},
	})

	podCreationErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bbroker_pod_creation_errors_total",
		Help: "Total pod creation failures.",
	}, []string{"reason"})

	cdpFrames = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bbroker_cdp_frames_total",
		Help: "Total CDP frames relayed.",
	}, []string{"direction"})

	sessionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bbroker_sessions_total",
		Help: "Total browser sessions created.",
	}, []string{"browser"})
)
