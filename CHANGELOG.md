# Changelog

## v0.1.0

Initial public release of Vexlo as a self-hosted localhost tunnel server.

- TCP, SSH, and WebSocket tunnel transports
- embedded terminal-style dashboard with live request updates
- request replay, mutation, and diffing
- SQLite persistence for sessions, requests, replays, and route rules
- registration-token protection for TCP and WebSocket clients
- SSH key allowlisting and persistent SSH host keys
- optional admin auth for dashboard and management APIs
- request size limits, retention pruning, health, and metrics endpoints
- Ubuntu `systemd` deployment artifacts and backup helper
