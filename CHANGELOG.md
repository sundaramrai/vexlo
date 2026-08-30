# Changelog

## Unreleased

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
