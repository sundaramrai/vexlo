# Changelog

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
