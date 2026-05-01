# Vexlo

Vexlo is a lightweight self-hosted localhost tunnel written in Go. It exposes a local port through a server, captures requests/responses, persists them to SQLite, and provides a terminal-style dashboard for replay, mutation, diffing, and route rules.

## What is included

- Binary TCP tunnel client/server
- SSH-backed tunnel transport, including native `ssh -R` remote forwarding
- Browser/WebSocket tunnel transport endpoint
- SQLite persistence for sessions, requests, replays, and routing rules
- Embedded dashboard assets with live updates over WebSocket
- Replay with request mutation
- JSON/text response diffing
- GitHub Actions release workflow

## Project layout

```text
cmd/server          Server entrypoint
cmd/client          Client CLI entrypoint
internal/server     HTTP/API/tunnel server implementation
internal/client     Tunnel client implementation
internal/model      Shared data models
internal/storage    SQLite layer
internal/dashboard  Embedded dashboard HTML/CSS/JS + websocket hub
internal/replay     Replay and diff logic
internal/protocol   Framed transport protocol
```

## Local development

### 1. Prerequisites

- Go 1.22 or newer

### 2. Run the server

```bash
go run ./cmd/server --http-addr :8080 --tcp-addr :9000 --ssh-addr :2222 --base-domain localhost --host-url http://localhost:8080 --capture-body-limit 262144
```

This starts:

- Dashboard/API on `http://localhost:8080`
- Binary tunnel listener on `127.0.0.1:9000`
- SSH tunnel listener on `127.0.0.1:2222`
- SQLite database at `./vexlo.db` by default, configurable with `--db`

By default, Vexlo stores up to `256 KiB` of each request and response body for dashboard history and replay. Use `--capture-body-limit 0` to disable the limit.

With defaults, this can usually be shortened to:

```bash
go run ./cmd/server
```

### 3. Run your local app

For example, start your app on port `3000`.

### 4. Start the Vexlo client

Binary mode:

```bash
go run ./cmd/client --server 127.0.0.1:9000 3000
```

With defaults, this can usually be shortened to:

```bash
go run ./cmd/client 3000
```

SSH mode:

```bash
go run ./cmd/client --mode ssh --ssh-addr 127.0.0.1:2222 3000
```

Native OpenSSH remote-forward mode:

```bash
ssh -N -R 80:localhost:3000 your-user@127.0.0.1 -p 2222
```

When the SSH session is accepted, Vexlo prints the assigned public URL and dashboard URL in the SSH output stream.

WebSocket mode:

```bash
go run ./cmd/client --mode ws --ws-url ws://127.0.0.1:8080/ws/tunnel 3000
```

The client prints:

- Public tunnel path, for local development this is `http://localhost:8080/t/<subdomain>`
- Dashboard URL with the session token baked in

### 5. Use the dashboard

Open the printed dashboard URL. From there you can:

- Inspect requests in real time
- Replay requests
- Mutate headers/body before replay
- View diff output
- Add basic routing rules

The dashboard source lives in:

- `internal/dashboard/dashboard.html`
- `internal/dashboard/dashboard.css`
- `internal/dashboard/dashboard.js`

## Build binaries

```bash
go build -o dist/vexlo-server ./cmd/server
go build -o dist/vexlo ./cmd/client
```

## Linting

Linting is enabled through GitHub Actions and `golangci-lint`.

Current checks:

- `gofmt` formatting check
- `golangci-lint`
- `go build ./...`

## Local checks

Install `golangci-lint` locally first:

<https://golangci-lint.run/welcome/install/>

Windows PowerShell:

```powershell
.\scripts\check.ps1
```

Linux or macOS:

```bash
./scripts/check.sh
```

Equivalent manual commands:

```bash
gofmt -l cmd internal
golangci-lint run
go build ./...
```

## Production notes

- For real public subdomains, run the server behind a DNS name and pass `--base-domain your-domain.example`.
- For TLS, start the server with `--tls`, expose ports `80` and `443`, and point your base domain plus subdomains at the VPS.
- The current local-first flow uses `/t/<subdomain>` when `--base-domain localhost`.
- Native `ssh -R` sessions proxy traffic correctly, but request routing to multiple target ports is still best served by the Vexlo CLI or WebSocket client because OpenSSH remote forwarding maps to a single local target.
- API responses include `X-Request-Id`, and server logs now emit request IDs plus status and duration metadata for API and WebSocket endpoints.
- Deployment artifacts for Oracle Ubuntu, `systemd`, and DuckDNS are in [deploy/README.md](deploy/README.md).

## Production server example

```bash
./vexlo-server \
  --tls \
  --http-addr :80 \
  --https-addr :443 \
  --tcp-addr :9000 \
  --ssh-addr :2222 \
  --base-domain vexlo.example.com \
  --host-url https://vexlo.example.com \
  --capture-body-limit 262144 \
  --acme-email you@example.com \
  --acme-cache ./acme-cache
```

## Native SSH usage against a public server

```bash
ssh -N -R 80:localhost:3000 vexlo@vexlo.example.com -p 2222
```

The server prints:

- `Public URL: https://<subdomain>.vexlo.example.com`
- `Dashboard: https://vexlo.example.com/?token=...&session=...`

## Oracle Cloud deployment

The repo includes ready-to-use deployment files:

- [deploy/systemd/vexlo.service](deploy/systemd/vexlo.service)
- [deploy/env/vexlo.env.example](deploy/env/vexlo.env.example)
- [deploy/scripts/install_oracle_ubuntu.sh](deploy/scripts/install_oracle_ubuntu.sh)
- [deploy/scripts/duckdns-update.sh](deploy/scripts/duckdns-update.sh)
- [deploy/scripts/install_duckdns_cron.sh](deploy/scripts/install_duckdns_cron.sh)

Typical flow on an Ubuntu VPS:

```bash
chmod +x deploy/scripts/install_oracle_ubuntu.sh
sudo ./deploy/scripts/install_oracle_ubuntu.sh \
  vexlo.example.com \
  https://vexlo.example.com \
  you@example.com \
  https://github.com/yourorg/vexlo/releases/latest/download/vexlo-server-linux-amd64
sudo systemctl start vexlo
sudo journalctl -u vexlo -f
```

If you use DuckDNS:

```bash
sudo nano /etc/vexlo/vexlo.env
chmod +x deploy/scripts/install_duckdns_cron.sh
sudo ./deploy/scripts/install_duckdns_cron.sh
```

## Verification

```bash
go build ./...
```
