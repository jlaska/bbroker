package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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
	defenderImage := flag.String("defender-image", "ghcr.io/jlaska/bbroker-defender:latest", "defender sidecar image")
	xvfbImage := flag.String("xvfb-image", "ghcr.io/jlaska/bbroker-xvfb:latest", "xvfb sidecar image")
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (empty = in-cluster)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	k8sClient, err := buildK8sClient(*kubeconfig)
	if err != nil {
		slog.Error("build k8s client", "err", err)
		os.Exit(1)
	}

	cfg := proxy.Config{
		ListenAddr:    *addr,
		MetricsAddr:   *metricsAddr,
		Namespace:     *namespace,
		BrowserImage:  *browserImage,
		DefenderImage: *defenderImage,
		XvfbImage:     *xvfbImage,
	}
	manager := proxy.NewSessionManager(k8sClient, *namespace, *browserImage, *defenderImage, *xvfbImage)
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
