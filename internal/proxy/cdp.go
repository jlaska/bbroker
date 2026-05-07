package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type cdpVersionResponse struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// DiscoverCDPEndpoint polls Chrome's /json/version endpoint until it responds
// or the timeout is reached. Returns the WebSocket debugger URL.
func DiscoverCDPEndpoint(podIP string, cdpPort int) (string, error) {
	url := fmt.Sprintf("http://%s:%d/json/version", podIP, cdpPort)
	deadline := time.Now().Add(30 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		var v cdpVersionResponse
		if err := json.Unmarshal(body, &v); err != nil || v.WebSocketDebuggerURL == "" {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return v.WebSocketDebuggerURL, nil
	}
	return "", fmt.Errorf("chrome CDP not ready on %s:%d after 30s", podIP, cdpPort)
}
