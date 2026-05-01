# Deployment

This directory contains production deployment artifacts for running Vexlo on an Ubuntu-based VPS such as Oracle Cloud free tier.

## Included files

- `systemd/vexlo.service`
  System service for the Vexlo server.
- `env/vexlo.env.example`
  Environment file template consumed by the service.
- `scripts/install_oracle_ubuntu.sh`
  Bootstraps packages, directories, firewall rules, env file, and systemd service.
- `scripts/duckdns-update.sh`
  Updates a DuckDNS record using environment variables.
- `scripts/install_duckdns_cron.sh`
  Installs a cron entry to refresh DuckDNS every 5 minutes.

## Expected production layout

- Binary: `/opt/vexlo/vexlo-server`
- Config: `/etc/vexlo/vexlo.env`
- Database: `/var/lib/vexlo/vexlo.db`
- ACME cache: `/var/lib/vexlo/acme-cache`

## Fast path

```bash
chmod +x deploy/scripts/install_oracle_ubuntu.sh
sudo ./deploy/scripts/install_oracle_ubuntu.sh \
  vexlo.example.com \
  https://vexlo.example.com \
  you@example.com \
  https://github.com/yourorg/vexlo/releases/latest/download/vexlo-server-linux-amd64
```

Then:

```bash
sudo systemctl start vexlo
sudo journalctl -u vexlo -f
```

If you use DuckDNS:

```bash
sudo nano /etc/vexlo/vexlo.env
sudo chmod +x deploy/scripts/install_duckdns_cron.sh
sudo ./deploy/scripts/install_duckdns_cron.sh
```
