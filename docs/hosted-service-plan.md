# Hosted Vexlo: quick tunnels

## Goal

On Windows, macOS, or Linux, a developer installs the Vexlo CLI with one
documented command for their OS, starts a local app, and runs:

```text
vexlo http 3000
```

Vexlo prints a temporary public HTTPS URL and a private dashboard link. The
developer does not create an account, obtain a token from the operator, install
Go, configure DNS, or run a server. The public URL works while the CLI is
connected and expires when the tunnel ends. The person operating hosted Vexlo
still maintains its server, DNS, TLS, capacity, and support.

This is the target for the next release. Today's published release and VPS
still use the old registration-token flow. The working tree removes that flow:
the CLI exposes only `vexlo http <port>`, and the new server accepts only
explicitly enabled hosted mode. Neither the released CLI nor the live service
provides this new flow yet.

## First release: temporary tunnels

- The hosted server accepts a new tunnel without a shared registration token.
  It assigns an unpredictable public URL and separate secrets for reconnecting
  and opening that tunnel's dashboard. The public URL alone grants no access
  to captured traffic or replay controls.
- Anyone with the public URL can reach the developer's local app. The CLI
  clearly says so. Stopping the CLI stops forwarding; any brief reconnect
  grace period and the maximum session lifetime have documented limits.
- Only the tunnel creator can inspect, replay, or mutate its requests. Hosted
  dashboard access is scoped to one tunnel. The browser must not receive a
  long-lived secret in a URL, and secrets must not appear in logs or be stored
  in plaintext on the server.
- Captured data expires after a short, published period. The service explains
  that it can see forwarded traffic and that a public tunnel should not expose
  a sensitive local app without additional protection.
- Anonymous registration is enabled only when the operator explicitly sets
  hosted mode. With the default disabled setting, the new server fails closed.
  There is no self-hosted compatibility path in the next binary.

## Minimum work before opening registration to everyone

- Bound registrations, active tunnels, concurrent requests, request and
  captured-body sizes, storage use, and session lifetime. Use fair limits for
  developers sharing one network and show useful errors when limits are hit.
- Verify that one tunnel cannot read, replay, or resume another tunnel's data.
  Use high-entropy per-tunnel secrets, expiry, and a safe browser handoff.
- Require verified TLS, monitor capacity and errors, provide an abuse contact,
  and give the operator a way to stop abusive tunnels or pause registrations.
- Test cleanup, restart recovery, and restoration of the production database
  backup. Choose the initial limits and retention period from a load test on
  the available VPS, then publish those values.

These controls affect the service behind the scenes; they add no signup steps
for developers. SQLite may remain while one instance meets measured capacity
and recovery needs. Accounts, OAuth, PostgreSQL, billing, and invite lists are
not prerequisites for temporary tunnels.

## Delivery order

1. **Agree on the behavior.** Specify the exact CLI output, when URLs expire,
   how reconnect works, dashboard access, and initial limits.
2. **Build the hosted path.** Add anonymous tunnel registration, per-tunnel
   dashboard authorization, expiry, limits, and cleanup. Test two independent
   clients, invalid secrets, resource limits, reconnect, and shutdown.
3. **Make it easy to install.** Add `vexlo http <port>`, publish one verified
   installation command for each supported OS, and test the full tunnel and
   dashboard flow on Windows, macOS, and Linux. Do not claim package-manager
   support until packages are actually published.
4. **Open access.** Check the service on the VPS, publish limits and a support
   contact, and allow anyone to start a temporary tunnel without an invite.
   Monitor usage and adjust limits from real demand.

The first release is done when a new developer on each supported OS can
install, run `vexlo http 3000`, send a request through the public URL, view it
privately, and stop the tunnel without contacting the operator.

## Implementation status (not a launch approval)

The working tree includes quick-mode registration, per-tunnel hashed secrets,
single-use dashboard handoff, scoped cookies, expiry and retention, rate and
concurrency limits, a pause/revoke operator API, cross-platform release builds,
and one-line installer endpoints. The token-based path has been removed from the working
tree. Deployment templates still set `VEXLO_HOSTED_MODE=false` as a fail-closed
default; the new binary will not start until the operator enables hosted mode.
Unit tests and local cross-compilation pass.

Before a public release, still verify a real tunnel and private dashboard on
Windows, macOS, and Linux; test installation from the *new* tagged release;
load-test the intended VPS and set published limits from those results; restore
and inspect a production backup; add monitoring and a support/abuse contact;
and deliberately update the VPS binary and service config. A local cross-build
does not substitute for those checks. Do not turn on anonymous registration
or advertise the installer as ready until they pass.

## Later, if needed

Accounts can provide stable URLs, saved history, and access across devices.
Visitor access controls, teams, paid plans, and a different database can follow
real product or capacity needs. Their design should not delay quick tunnels.

## References

- [Cloudflare Quick Tunnels](https://developers.cloudflare.com/tunnel/get-started/quick-tunnels/)
  demonstrate a temporary, no-account tunnel flow.
- [OWASP Denial of Service Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html)
  covers service resource and rate limits.
