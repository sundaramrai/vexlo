# Deployment

This directory contains production deployment artifacts for running Vexlo on an Ubuntu-based VPS such as Oracle Cloud free tier.

## Included files

- `systemd/vexlo.service`
  System service for the Vexlo server.
- `env/vexlo.env.example`
  Environment file template consumed by the service.
- `scripts/install_oracle_ubuntu.sh`
  Bootstraps packages, directories, firewall rules, env file, and systemd service.
- `scripts/backup_vexlo.sh`
  Copies the SQLite database and SSH host key into a timestamped backup directory.
- `scripts/duckdns-update.sh`
  Updates a DuckDNS record using environment variables.
- `scripts/install_duckdns_cron.sh`
  Installs a cron entry to refresh DuckDNS every 5 minutes.

## Expected production layout

- Binary: `/opt/vexlo/vexlo-server`
- Config: `/etc/vexlo/vexlo.env`
- SSH allowlist: `/etc/vexlo/authorized_keys`
- SSH host key: `/etc/vexlo/ssh_host_key`
- Database: `/var/lib/vexlo/vexlo.db`
- ACME cache: `/var/lib/vexlo/acme-cache`
- Backup helper: `/opt/vexlo/backup_vexlo.sh`

## Fast path

```bash
chmod +x deploy/scripts/install_oracle_ubuntu.sh
sudo ./deploy/scripts/install_oracle_ubuntu.sh \
  vexlo.example.com \
  https://vexlo.example.com \
  you@example.com \
  https://github.com/yourorg/vexlo/releases/latest/download/vexlo-server-linux-amd64.tar.gz
```

Then:

```bash
sudo systemctl start vexlo
sudo journalctl -u vexlo -f
```

Before exposing the service publicly:

```bash
sudo nano /etc/vexlo/vexlo.env
sudo nano /etc/vexlo/authorized_keys
```

At minimum, replace:

- `VEXLO_REGISTRATION_TOKEN`
- `VEXLO_ADMIN_PASS`
- `VEXLO_ADMIN_USER` if you do not want the default `admin`

And append the SSH public keys that are allowed to open SSH-backed tunnels.

If you use DuckDNS:

```bash
sudo nano /etc/vexlo/vexlo.env
sudo chmod +x deploy/scripts/install_duckdns_cron.sh
sudo ./deploy/scripts/install_duckdns_cron.sh
```

## Minimal production launch shape

The installed `systemd` unit runs the server with:

- TLS enabled on `:80` and `:443`
- TCP tunnel listener on `:9000`
- SSH tunnel listener on `:2222`
- explicit registration token and admin credentials from `/etc/vexlo/vexlo.env`
- persistent DB, ACME cache, and SSH host key paths
- body-size limits, retention, and HTTP timeouts

Equivalent manual command:

```bash
/opt/vexlo/vexlo-server \
  --tls \
  --http-addr :80 \
  --https-addr :443 \
  --tcp-addr :9000 \
  --ssh-addr :2222 \
  --base-domain "$VEXLO_BASE_DOMAIN" \
  --host-url "$VEXLO_HOST_URL" \
  --registration-token "$VEXLO_REGISTRATION_TOKEN" \
  --admin-user "$VEXLO_ADMIN_USER" \
  --admin-pass "$VEXLO_ADMIN_PASS" \
  --allowed-ssh-keys /etc/vexlo/authorized_keys \
  --ssh-host-key /etc/vexlo/ssh_host_key \
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
- `/etc/vexlo/ssh_host_key`
