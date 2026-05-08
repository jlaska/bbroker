# Browser Broker (bbroker)

Kubernetes-native browser-as-a-service. Creates isolated Chrome browser pods on demand for CDP/Playwright/Puppeteer clients.

## Architecture

- **bbrokerd** — stateless WebSocket proxy. Each client connection creates an ephemeral browser pod and relays CDP traffic directly to it. No database, no session store.
- **bbroker-warden** — sidecar in every browser pod. Enforces single-session, manages idle/session timeouts, self-terminates the pod via k8s API.
- **bbroker-xvfb** — optional sidecar for headful Chrome (virtual display). Added when `?headful=true` is in the URL.

Sessions = pods. `kubectl get pods -l bbroker.io/component=browser` shows all active sessions.

## Quick Start

```bash
# Install with Helm
helm repo add bbroker https://jlaska.github.io/bbroker
helm install bbroker bbroker/bbroker --namespace bbroker-system --create-namespace

# Connect via Playwright
const browser = await chromium.connectOverCDP('ws://bbrokerd.bbroker-system.svc.cluster.local:4444/cdtp/chrome');

# Headful mode (for reCAPTCHA, etc.)
const browser = await chromium.connectOverCDP('ws://bbrokerd...:4444/cdtp/chrome?headful=true');
```

## Endpoints

| Path | Protocol | Description |
|------|----------|-------------|
| `/cdtp/chrome` | WebSocket | CDP relay — creates a browser pod |
| `/status` | HTTP GET | Active sessions JSON |
| `/health` | HTTP GET | Health check |
| `:8080/metrics` | HTTP GET | Prometheus metrics |

## Migration from sockpuppetbrowser

| Consumer | Old URL | New URL |
|----------|---------|---------|
| changedetection | `ws://sockpuppetbrowser:3000` | `ws://bbrokerd:4444/cdtp/chrome` |
| yosemite-checker | `ws://sockpuppetbrowser:3000/?headful=true` | `ws://bbrokerd:4444/cdtp/chrome?headful=true` |

## Configuration

See `charts/bbroker/values.yaml` for all options. Key values:

```yaml
browser:
  image: chromedp/headless-shell:latest
  idleTimeoutSeconds: 300
  sessionTimeoutSeconds: 1800

replicaCount: 2   # bbrokerd replicas (stateless, scale freely)
```

## Development

```bash
make build          # build binaries to bin/
make test           # run tests
make vet            # go vet
make docker-build   # build all Docker images
make helm-lint      # lint Helm chart
```

## License

Apache 2.0
