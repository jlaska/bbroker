package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	bk8s "github.com/jlaska/bbroker/internal/k8s"
	"github.com/jlaska/bbroker/internal/proxy"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	addr := flag.String("addr", ":4444", "listen address")
	metricsAddr := flag.String("metrics-addr", ":8080", "metrics/health listen address")
	namespace := flag.String("namespace", "bbroker-system", "namespace to create browser pods in")
	browserImage := flag.String("browser-image", "chromedp/headless-shell:latest", "browser container image")
	wardenImage := flag.String("warden-image", "ghcr.io/jlaska/bbroker-warden:latest", "warden sidecar image")
	xvfbImage := flag.String("xvfb-image", "ghcr.io/jlaska/bbroker-xvfb:latest", "xvfb sidecar image")
	directWSPort := flag.Int("direct-ws-port", 0, "skip CDP discovery and connect directly to this port (for proxy images like sockpuppetbrowser)")
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (empty = in-cluster)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	k8sClient, err := buildK8sClient(*kubeconfig)
	if err != nil {
		slog.Error("build k8s client", "err", err)
		os.Exit(1)
	}

	// Clean up any browser pods left over from a previous proxy crash.
	startupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := bk8s.CleanupOrphanedPods(startupCtx, k8sClient, *namespace); err != nil {
		slog.Warn("orphan cleanup", "err", err)
	}
	cancel()

	cfg := proxy.Config{
		ListenAddr:   *addr,
		MetricsAddr:  *metricsAddr,
		Namespace:    *namespace,
		BrowserImage: *browserImage,
		WardenImage:  *wardenImage,
		XvfbImage:    *xvfbImage,
		DirectWSPort: *directWSPort,
	}
	manager := proxy.NewSessionManager(k8sClient, cfg)
	srv := proxy.NewServer(cfg, manager)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		slog.Error("bbrokerd exited", "err", err)
		os.Exit(1)
	}
}

func buildK8sClient(kubeconfig string) (kubernetes.Interface, error) {
	var cfg *rest.Config
	var err error
	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}
