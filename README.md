# Vexlo

Vexlo gives a local development server a temporary public HTTPS URL and a
private dashboard for inspecting, replaying, and mutating HTTP requests.

## Status

Vexlo v0.2.1 is released, and the hosted-only server is running at
`vexlo.duckdns.org`. The CLI accepts `vexlo http <port>` without an
operator-issued registration token. The Windows one-line install and a real
tunnel have been tested against the live service. On-device macOS and Linux
install-and-tunnel checks and the remaining operational checks are still
tracked in [the hosted-service plan](docs/hosted-service-plan.md).

## Install and use

Install with one command for your shell:

macOS / Linux:

```sh
curl -fsSL https://vexlo.duckdns.org/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://vexlo.duckdns.org/install.ps1 | iex
```

The installer detects the platform and architecture and verifies the release
archive against its published SHA-256 checksum. It does not require administrator
privileges. On systems without a writable executable directory already on PATH,
open a new terminal after installation; the installer sets up PATH automatically.
Start your local app on port 3000, then run:

```text
vexlo http 3000
```

The CLI prints a temporary public URL and a private dashboard link. Anyone
with the public URL can reach your local app while the CLI is connected. Keep
the dashboard link private. No account or operator-issued token is required.

The tunnel lasts at most eight hours, with a brief reconnect grace period.
Captured data is deleted after the configured hosted retention period. The
hosted operator can inspect traffic passing through the service; do not expose
a sensitive local app without additional protection.

## For contributors and operators

- [Deployment guide](deploy/README.md) explains the VPS service, TLS, explicit
  hosted activation, backups, and operator controls.
- [Release guide](RELEASE.md) describes verification before tagging.
- [Hosted-service plan](docs/hosted-service-plan.md) tracks launch gates.

The code is organized under `cmd/client`, `cmd/server`, and `internal/` for the
CLI, server, protocol, dashboard, and SQLite storage. Go 1.25.13 or newer is
required to build it:

```bash
go test ./...
go vet ./...
go build ./...
```

Release artifacts are built by [.github/workflows/release.yml](.github/workflows/release.yml)
for Linux, macOS, and Windows on amd64 and arm64 (the server is packaged for
Linux). Historical releases remain documented in [CHANGELOG.md](CHANGELOG.md).
