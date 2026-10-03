# Changelog

## v0.2.1

- fix the Windows one-line installer when PowerShell reports no OS architecture
- test the installer in Windows PowerShell 5.1 and cover the missing-architecture fallback

## v0.2.0

- replace the shared registration-token flow with hosted quick tunnels and
  `vexlo http <port>`
- scope dashboard access and reconnect credentials to individual tunnels
- add bounded tunnel lifetimes, rate and concurrency limits, and operator controls
- publish one-line Windows, macOS, and Linux installers with verified release checksums
- make hosted mode an explicit server setting with a fail-closed default

## v0.1.7

- upgrade `modernc.org/sqlite` from v1.37.1 to v1.58.0

## v0.1.6

- upgrade GitHub Actions checkout and Go setup actions to v7
- upgrade `github.com/coder/websocket` to v1.8.15

## v0.1.5

- add a public landing page at `/`
- move the protected dashboard to `/app`
- preserve legacy dashboard links by redirecting them to `/app`
- keep dashboard assets protected while serving the landing stylesheet publicly

## v0.1.4

- upgrade the Go toolchain to 1.25.13 to remediate reachable standard-library
  vulnerabilities
- upgrade `golang.org/x/crypto`, `golang.org/x/net`, `golang.org/x/sys`, and
  `golang.org/x/text` to patched versions
- pin the CI vulnerability checker for reproducible scans
- update release and deployment documentation to match the current TLS and
  secret-handling design

## v0.1.3

- load registration and dashboard credentials from the protected service
  environment instead of exposing them in process arguments

## v0.1.2

- require TLS for the production tunnel transport
- support separate dashboard and wildcard TLS certificate pairs
- sync renewed certificates to files readable by the unprivileged service
- add safer Windows local-demo startup behavior

## v0.1.1

Release maintenance update.

- switched module path to `github.com/sundaramrai/vexlo`
- fixed GitHub release workflow permissions
- updated the GitHub release action to `softprops/action-gh-release@v3`
- trimmed and clarified top-level documentation

## v0.1.0

Initial public release of Vexlo as a self-hosted localhost tunnel server.

- TCP tunnel transport
- embedded terminal-style dashboard with live request updates
- request replay and mutation
- SQLite persistence for sessions, requests, and replays
- registration-token protection for tunnel clients
- optional admin auth for dashboard and management APIs
- request size limits, retention pruning, and a health endpoint
- Ubuntu `systemd` deployment artifacts and backup helper
