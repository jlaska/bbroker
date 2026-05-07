package k8s

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// WaitForPodReady blocks until the named pod is Ready or ctx is cancelled.
// Returns the pod's IP on success.
func WaitForPodReady(ctx context.Context, client kubernetes.Interface, namespace, name string) (string, error) {
	// Poll with a watcher to avoid busy-looping.
	watcher, err := client.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", name),
	})
	if err != nil {
		return "", fmt.Errorf("watch pod %s: %w", name, err)
	}
	defer watcher.Stop()

	timeout := time.NewTimer(60 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeout.C:
			return "", fmt.Errorf("timed out waiting for pod %s to be ready", name)
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return "", fmt.Errorf("watch channel closed for pod %s", name)
			}
			if event.Type == watch.Deleted {
				return "", fmt.Errorf("pod %s was deleted while waiting for ready", name)
			}
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			if pod.Status.Phase == corev1.PodFailed {
				return "", fmt.Errorf("pod %s failed: %s", name, pod.Status.Message)
			}
			if isPodReady(pod) {
				return pod.Status.PodIP, nil
			}
		}
	}
}

func isPodReady(pod *corev1.Pod) bool {
	if pod.Status.PodIP == "" {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
