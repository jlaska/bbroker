package k8s

import (
	"context"
	"fmt"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	LabelManagedBy = "bbroker.io/managed-by"
	LabelComponent = "bbroker.io/component"
	LabelSessionID = "bbroker.io/session-id"

	ComponentBrowser = "browser"

	WardenPort = 4545
	CDPPort      = 9222
)

// SessionConfig holds the parameters for creating a browser pod.
type SessionConfig struct {
	SessionID    string
	Namespace    string
	BrowserImage string
	// BrowserArgs overrides the browser container's command args.
	// When empty, the image's own entrypoint runs unmodified (correct for
	// self-contained images like chromedp/headless-shell that manage Chrome
	// startup internally). Set explicitly for bare Chrome binaries.
	BrowserArgs  []string
	WardenImage  string
	Headful      bool
	XvfbImage    string
	Params       url.Values
}

// CreateBrowserPod creates an ephemeral browser pod and returns its name.
func CreateBrowserPod(ctx context.Context, client kubernetes.Interface, cfg SessionConfig) (*corev1.Pod, error) {
	pod := buildPodSpec(cfg)
	return client.CoreV1().Pods(cfg.Namespace).Create(ctx, pod, metav1.CreateOptions{})
}

// DeletePod deletes a pod by name.
func DeletePod(ctx context.Context, client kubernetes.Interface, namespace, name string) error {
	gracePeriod := int64(30)
	return client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	})
}

// ListBrowserPods returns all browser pods managed by bbroker.
func ListBrowserPods(ctx context.Context, client kubernetes.Interface, namespace string) (*corev1.PodList, error) {
	return client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", LabelComponent, ComponentBrowser),
	})
}

func buildPodSpec(cfg SessionConfig) *corev1.Pod {
	// Use explicit BrowserArgs if provided; otherwise let the image's own
	// entrypoint handle Chrome startup (correct for headless-shell, etc.).
	args := cfg.BrowserArgs
	if args == nil && len(cfg.Params) > 0 {
		args = buildChromeArgs(cfg)
	}
	containers := []corev1.Container{
		browserContainer(cfg, args),
		wardenContainer(cfg),
	}
	if cfg.Headful {
		containers = append(containers, xvfbContainer(cfg))
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.SessionID,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				LabelManagedBy: "bbroker",
				LabelComponent: ComponentBrowser,
				LabelSessionID: cfg.SessionID,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: "bbroker-browser",
			Containers:         containers,
			Volumes: []corev1.Volume{
				{
					Name: "dshm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium:    corev1.StorageMediumMemory,
							SizeLimit: resourcePtr("256Mi"),
						},
					},
				},
			},
		},
	}
	return pod
}

func browserContainer(cfg SessionConfig, chromeArgs []string) corev1.Container {
	c := corev1.Container{
		Name:  "browser",
		Image: cfg.BrowserImage,
		Args:  chromeArgs,
		Ports: []corev1.ContainerPort{
			{Name: "cdp", ContainerPort: CDPPort, Protocol: corev1.ProtocolTCP},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "dshm", MountPath: "/dev/shm"},
		},
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_ADMIN"},
			},
		},
	}
	if cfg.Headful {
		c.Env = append(c.Env, corev1.EnvVar{Name: "DISPLAY", Value: ":99"})
	}
	return c
}

func wardenContainer(cfg SessionConfig) corev1.Container {
	env := []corev1.EnvVar{
		{Name: "BBROKER_CDP_PORT", Value: fmt.Sprintf("%d", CDPPort)},
		// Downward API: pod name for self-termination
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
			},
		},
		{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
			},
		},
	}
	return corev1.Container{
		Name:  "warden",
		Image: cfg.WardenImage,
		Ports: []corev1.ContainerPort{
			{Name: "warden", ContainerPort: WardenPort, Protocol: corev1.ProtocolTCP},
		},
		Env: env,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
	}
}

func xvfbContainer(cfg SessionConfig) corev1.Container {
	return corev1.Container{
		Name:  "xvfb",
		Image: cfg.XvfbImage,
		Env: []corev1.EnvVar{
			{Name: "DISPLAY_NUM", Value: "99"},
			{Name: "SCREEN_WIDTH", Value: "1920"},
			{Name: "SCREEN_HEIGHT", Value: "1080"},
			{Name: "SCREEN_DEPTH", Value: "24"},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
	}
}

func buildChromeArgs(cfg SessionConfig) []string {
	args := []string{
		"--remote-debugging-address=0.0.0.0",
		fmt.Sprintf("--remote-debugging-port=%d", CDPPort),
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--disable-gpu",
	}
	if !cfg.Headful {
		args = append(args, "--headless=new")
	}
	// Pass through any extra flags from query params (e.g. --window-size)
	for key := range cfg.Params {
		if len(key) > 2 && key[:2] == "--" {
			val := cfg.Params.Get(key)
			if val == "" {
				args = append(args, key)
			} else {
				args = append(args, fmt.Sprintf("%s=%s", key, val))
			}
		}
	}
	return args
}

func resourcePtr(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}
