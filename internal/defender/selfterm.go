package defender

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// PodTerminator deletes the pod it's running in via the k8s API.
type PodTerminator struct {
	client    kubernetes.Interface
	podName   string
	namespace string
}

// NewPodTerminator creates a PodTerminator using in-cluster config and
// the POD_NAME / POD_NAMESPACE env vars injected by the downward API.
func NewPodTerminator() (*PodTerminator, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("k8s client: %w", err)
	}
	podName := os.Getenv("POD_NAME")
	namespace := os.Getenv("POD_NAMESPACE")
	if podName == "" || namespace == "" {
		return nil, fmt.Errorf("POD_NAME and POD_NAMESPACE env vars must be set")
	}
	return &PodTerminator{client: client, podName: podName, namespace: namespace}, nil
}

func (t *PodTerminator) Terminate(ctx context.Context) error {
	gracePeriod := int64(0)
	return t.client.CoreV1().Pods(t.namespace).Delete(ctx, t.podName, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	})
}
