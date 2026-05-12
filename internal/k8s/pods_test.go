package k8s

import (
	"net/url"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestBuildPodSpec_Headless(t *testing.T) {
	cfg := SessionConfig{
		SessionID:     "session-test",
		Namespace:     "bbroker-system",
		BrowserImage:  "chromedp/headless-shell:latest",
		WardenImage: "ghcr.io/jlaska/bbroker-warden:latest",
		Headful:       false,
		XvfbImage:     "ghcr.io/jlaska/bbroker-xvfb:latest",
		Params:        url.Values{},
	}
	pod := buildPodSpec(cfg)

	if pod.Name != "session-test" {
		t.Errorf("pod name = %q", pod.Name)
	}
	if pod.Labels[LabelComponent] != ComponentBrowser {
		t.Error("missing component label")
	}
	if len(pod.Spec.Containers) != 2 {
		t.Errorf("expected 2 containers (browser+warden), got %d", len(pod.Spec.Containers))
	}
	// Headless: no --headless args override needed but no DISPLAY env
	for _, c := range pod.Spec.Containers {
		if c.Name == "browser" {
			for _, env := range c.Env {
				if env.Name == "DISPLAY" {
					t.Error("DISPLAY env set in headless mode")
				}
			}
		}
	}
}

func TestBuildPodSpec_Headful(t *testing.T) {
	cfg := SessionConfig{
		SessionID:     "session-headful",
		Namespace:     "bbroker-system",
		BrowserImage:  "chromedp/headless-shell:latest",
		WardenImage: "ghcr.io/jlaska/bbroker-warden:latest",
		Headful:       true,
		XvfbImage:     "ghcr.io/jlaska/bbroker-xvfb:latest",
		Params:        url.Values{},
	}
	pod := buildPodSpec(cfg)

	if len(pod.Spec.Containers) != 3 {
		t.Errorf("expected 3 containers (browser+warden+xvfb), got %d", len(pod.Spec.Containers))
	}

	hasXvfb := false
	hasSysAdmin := false
	for _, c := range pod.Spec.Containers {
		if c.Name == "xvfb" {
			hasXvfb = true
		}
		if c.Name == "browser" {
			for _, env := range c.Env {
				if env.Name == "DISPLAY" && env.Value == ":99" {
					hasSysAdmin = true
				}
			}
		}
	}
	if !hasXvfb {
		t.Error("xvfb container missing in headful mode")
	}
	if !hasSysAdmin {
		t.Error("DISPLAY=:99 not set on browser container in headful mode")
	}
}

func TestBuildPodSpec_SYSAdmin(t *testing.T) {
	cfg := SessionConfig{
		SessionID:    "session-caps",
		BrowserImage: "chromedp/headless-shell:latest",
		Params:       url.Values{},
	}
	pod := buildPodSpec(cfg)
	for _, c := range pod.Spec.Containers {
		if c.Name == "browser" {
			if c.SecurityContext == nil || c.SecurityContext.Capabilities == nil {
				t.Fatal("expected SecurityContext.Capabilities")
			}
			found := false
			for _, cap := range c.SecurityContext.Capabilities.Add {
				if cap == corev1.Capability("SYS_ADMIN") {
					found = true
				}
			}
			if !found {
				t.Error("SYS_ADMIN capability not set on browser container")
			}
		}
	}
}

func TestBuildChromeArgs_ExtraFlags(t *testing.T) {
	params := url.Values{}
	params.Set("--window-size", "1280,720")
	params.Set("headful", "true") // not a flag, should be ignored

	args := BuildChromeArgs(false, params)

	found := false
	for _, a := range args {
		if a == "--window-size=1280,720" {
			found = true
		}
		if a == "headful=true" || a == "--headful=true" {
			t.Error("non-flag param passed as Chrome arg")
		}
	}
	if !found {
		t.Error("--window-size not passed to Chrome")
	}
}

func TestBuildChromeArgs_Headless(t *testing.T) {
	args := BuildChromeArgs(false, url.Values{})
	found := false
	for _, a := range args {
		if a == "--headless=new" {
			found = true
		}
	}
	if !found {
		t.Error("expected --headless=new for headless mode")
	}
}

func TestBuildChromeArgs_Headful(t *testing.T) {
	args := BuildChromeArgs(true, url.Values{})
	for _, a := range args {
		if a == "--headless=new" || a == "--headless" {
			t.Errorf("unexpected headless flag in headful mode: %s", a)
		}
	}
	// Must still have CDP flags
	hasDebugAddr := false
	for _, a := range args {
		if a == "--remote-debugging-address=0.0.0.0" {
			hasDebugAddr = true
		}
	}
	if !hasDebugAddr {
		t.Error("expected --remote-debugging-address in headful args")
	}
}
