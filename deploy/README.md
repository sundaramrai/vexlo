# Deployment

This directory contains deployment artifacts for the operator-managed hosted
Vexlo server on a Linux VPS. Since v0.2.0 the server supports only hosted
tunnels. The deployment template keeps `VEXLO_HOSTED_MODE=false` as a
fail-closed default; the live `vexlo.duckdns.org` service has hosted mode
enabled. A new deployment must enable it deliberately.

## Included files

- `systemd/vexlo.service`
  System service for the Vexlo server.
- `env/vexlo.env.example`
  Environment file template consumed by the service.
- `scripts/install_ubuntu.sh`
  Bootstraps packages, directories, firewall rules, env file, and systemd service.
- `scripts/backup_vexlo.sh`
  Takes a consistent online SQLite backup and checks its integrity.

## Expected production layout

- Binary: `/opt/vexlo/vexlo-server`
- Config: `/etc/vexlo/vexlo.env`
- Database: `/var/lib/vexlo/vexlo.db`
- ACME cache: `/var/lib/vexlo/acme-cache`
- Backup helper: `/opt/vexlo/backup_vexlo.sh`
- Certificate sync helper: `/opt/vexlo/sync_certificates.sh`

## Fast path

```bash
chmod +x deploy/scripts/install_ubuntu.sh
sudo ./deploy/scripts/install_ubuntu.sh \
  vexlo.example.com \
  https://vexlo.example.com \
  you@example.com \
  https://github.com/OWNER/REPO/releases/download/vX.Y.Z/vexlo-server-linux-amd64.tar.gz \
  https://github.com/OWNER/REPO/releases/download/vX.Y.Z/SHA256SUMS.txt
```

Replace the example hostname, repository, release tag, and architecture with
values for your deployment. This is an operator example for a new VPS. Edit
`/etc/vexlo/vexlo.env` before starting the new server:

```bash
sudo nano /etc/vexlo/vexlo.env
```

Set strong `VEXLO_ADMIN_PASS` credentials and `VEXLO_HOSTED_MODE=true` only
when the deployment is ready to accept public tunnels. The binary has no
registration-token setting or legacy token-based tunnel mode. A service configured with
`VEXLO_HOSTED_MODE=false` will fail closed rather than start a public listener.
Ensure dashboard and wildcard certificate paths are valid, then start and
inspect the service:

```bash
sudo systemctl start vexlo
sudo systemctl --no-pager --full status vexlo
sudo journalctl -u vexlo -f
```

The service reads operator credentials from its protected environment file,
not from process arguments. Hosted mode requires HTTPS, tunnel TLS, a public
domain, positive limits, and operator credentials. The CLI currently targets
`vexlo.duckdns.org:9000`; a different deployment hostname requires a client
build configured for that service.

When hosted mode is deliberately enabled, a quick tunnel lasts at most 8 hours;
the reconnect grace is 30 seconds; ended captures are kept for 1 hour. The
initial code defaults to 100 active tunnels, 5 per source IP, 100 in-flight
public requests, 2 MiB request bodies, 16 KiB captured bodies, and 200 captured
requests and replays per tunnel. Registration and public request rate limits
also apply. These are provisional limits, not measured VPS capacity numbers.

The operator can pause new registrations or revoke one hosted tunnel using
the Basic-auth API (the password is prompted, not put in shell history):

```bash
curl -u admin -H 'Origin: https://vexlo.duckdns.org' \
  -H 'Content-Type: application/json' -d '{"paused":true}' \
  https://vexlo.duckdns.org/api/admin/registrations
curl -u admin -H 'Origin: https://vexlo.duckdns.org' \
  -X DELETE https://vexlo.duckdns.org/api/admin/tunnels/SESSION_ID
```

Use `{"paused":false}` to resume registrations. Replace the hostname, admin
username, and session ID for your deployment. These endpoints require HTTPS
and a matching `Origin` header.

For dynamic public tunnel subdomains, use a wildcard certificate obtained
through DNS-01. Set `VEXLO_TLS_CERT` and `VEXLO_TLS_KEY` to a certificate for
the dashboard hostname. Set `VEXLO_TLS_EXTRA_CERT` and `VEXLO_TLS_EXTRA_KEY`
to a wildcard certificate when the DNS provider cannot validate the dashboard
hostname and wildcard in one certificate request (DuckDNS is one such provider).
Each certificate/key pair must be complete. Vexlo selects the right certificate
from SNI for HTTPS and tunnel TLS.

The Vexlo service runs as an unprivileged user, so it reads root-synced copies
from `/etc/vexlo/certs`, rather than Let’s Encrypt's root-only directories.
After obtaining or renewing the `vexlo-dashboard` and `vexlo-wildcard`
certificates, run:

```bash
sudo /opt/vexlo/sync_certificates.sh
```

Register that same helper as Certbot's deploy hook so a successful renewal
restarts Vexlo with the updated certificates:

```bash
sudo install -d -m 755 /etc/letsencrypt/renewal-hooks/deploy
sudo ln -sf /opt/vexlo/sync_certificates.sh /etc/letsencrypt/renewal-hooks/deploy/vexlo-certificates
```

## Minimal production launch shape

The installed `systemd` unit runs the server with:

- TLS enabled on `:80` and `:443`
- TLS required for the public tunnel listener on `:9000`; the CLI verifies the hosted server name
- TCP tunnel listener on `:9000`
- explicit hosted activation and admin credentials from `/etc/vexlo/vexlo.env`
- persistent DB and ACME cache paths
- body-size limits, retention, and HTTP timeouts

Use the systemd unit for production. Do not pass the admin password as a CLI
flag in an interactive shell: process arguments can be visible to other users.
The service reads it from the protected environment file instead.

## Backups

Create an on-demand backup:

```bash
sudo /opt/vexlo/backup_vexlo.sh
```

That captures a consistent snapshot of `/var/lib/vexlo/vexlo.db`, including
committed WAL transactions. The helper requires `sqlite3`; do not copy a live
WAL database and its sidecar files separately. Before relying on backups for
recovery, restore one into a separate test directory and verify both integrity
and the expected session/request data. A VPS restore has not yet been verified.
