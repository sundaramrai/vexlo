# Deployment

This directory contains production deployment artifacts for running Vexlo on a Linux VPS.

## Included files

- `systemd/vexlo.service`
  System service for the Vexlo server.
- `env/vexlo.env.example`
  Environment file template consumed by the service.
- `scripts/install_ubuntu.sh`
  Bootstraps packages, directories, firewall rules, env file, and systemd service.
- `scripts/backup_vexlo.sh`
  Copies the SQLite database into a timestamped backup directory.

## Expected production layout

- Binary: `/opt/vexlo/vexlo-server`
- Config: `/etc/vexlo/vexlo.env`
- Database: `/var/lib/vexlo/vexlo.db`
- ACME cache: `/var/lib/vexlo/acme-cache`
- Backup helper: `/opt/vexlo/backup_vexlo.sh`

## Fast path

```bash
chmod +x deploy/scripts/install_ubuntu.sh
sudo ./deploy/scripts/install_ubuntu.sh \
  vexlo.example.com \
  https://vexlo.example.com \
  you@example.com \
  https://github.com/yourorg/vexlo/releases/latest/download/vexlo-server-linux-amd64.tar.gz \
  https://github.com/yourorg/vexlo/releases/latest/download/SHA256SUMS.txt
```

Then:

```bash
sudo systemctl start vexlo
sudo journalctl -u vexlo -f
```

Before exposing the service publicly:

```bash
sudo nano /etc/vexlo/vexlo.env
```

At minimum, replace:

- `VEXLO_REGISTRATION_TOKEN`
- `VEXLO_ADMIN_PASS`
- `VEXLO_ADMIN_USER` if you do not want the default `admin`

For dynamic public tunnel subdomains, use a wildcard certificate obtained
through DNS-01. Add `--tls-cert /path/to/fullchain.pem --tls-key
/path/to/privkey.pem` to the systemd `ExecStart` (or an override) before
starting Vexlo. Do not set only one of the two flags.

## Minimal production launch shape

The installed `systemd` unit runs the server with:

- TLS enabled on `:80` and `:443`
- TLS required for the public tunnel listener on `:9000`; clients must pass `--tls --server-name "$VEXLO_BASE_DOMAIN"`
- TCP tunnel listener on `:9000`
- explicit registration token and admin credentials from `/etc/vexlo/vexlo.env`
- persistent DB and ACME cache paths
- body-size limits, retention, and HTTP timeouts

Equivalent manual command:

```bash
/opt/vexlo/vexlo-server \
  --tls \
  --http-addr :80 \
  --https-addr :443 \
  --tcp-addr :9000 \
  --base-domain "$VEXLO_BASE_DOMAIN" \
  --host-url "$VEXLO_HOST_URL" \
  --registration-token "$VEXLO_REGISTRATION_TOKEN" \
  --admin-user "$VEXLO_ADMIN_USER" \
  --admin-pass "$VEXLO_ADMIN_PASS" \
  --capture-body-limit "$VEXLO_CAPTURE_BODY_LIMIT" \
  --max-request-body-bytes "$VEXLO_MAX_REQUEST_BODY_BYTES" \
  --max-api-body-bytes "$VEXLO_MAX_API_BODY_BYTES" \
  --retention-period "$VEXLO_RETENTION_PERIOD" \
  --read-timeout "$VEXLO_READ_TIMEOUT" \
  --write-timeout "$VEXLO_WRITE_TIMEOUT" \
  --idle-timeout "$VEXLO_IDLE_TIMEOUT" \
  --acme-email "$VEXLO_ACME_EMAIL" \
  --acme-cache /var/lib/vexlo/acme-cache \
  --db /var/lib/vexlo/vexlo.db
```

## Backups

Create an on-demand backup:

```bash
sudo /opt/vexlo/backup_vexlo.sh
```

That captures:

- `/var/lib/vexlo/vexlo.db`
- optional WAL/SHM files if present
