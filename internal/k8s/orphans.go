package k8s

import (
	"context"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CleanupOrphanedPods deletes any browser pods left over from a previous proxy
// crash. Called once on bbrokerd startup. Orphans have the bbroker component
// label but no active proxy connection tracking them.
func CleanupOrphanedPods(ctx context.Context, client kubernetes.Interface, namespace string) error {
	pods, err := ListBrowserPods(ctx, client, namespace)
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return nil
	}
	slog.Warn("cleaning up orphaned browser pods", "count", len(pods.Items))
	for _, pod := range pods.Items {
		gracePeriod := int64(0)
		if err := client.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
		}); err != nil {
			slog.Error("delete orphaned pod", "pod", pod.Name, "err", err)
		} else {
			slog.Info("deleted orphaned pod", "pod", pod.Name)
		}
	}
	return nil
}
