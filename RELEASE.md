# Release Guide

The next release changes Vexlo to a hosted-only tunnel service and CLI. Do not
tag it until the hosted launch checks are complete and the server deployment is
ready. Existing published releases retain their historical behavior.

## Scope

- `vexlo-server`: operator-managed hosted server binary
- `vexlo`: developer CLI (`vexlo http <port>`)

## Before Tagging

1. Confirm the working tree is in the state you want to publish.
2. Run local verification:

```bash
go test ./...
go vet ./...
go build ./...
```

Run `sh scripts/test_install.sh` on Linux or macOS. CI also runs the Windows
installer fixture test on a disposable Windows runner. Do not run that test on
your everyday Windows account: it temporarily updates the user PATH.

1. Read [README.md](README.md) and [deploy/README.md](deploy/README.md) once as a user, not as the author.
2. Complete the remaining checks in [the hosted-service plan](docs/hosted-service-plan.md): real cross-platform tunnels and dashboard access, installation from the tagged artifacts, load limits, backup restoration, and monitoring/abuse contact. Confirm all installer CI jobs pass.
3. Update [CHANGELOG.md](CHANGELOG.md) for the breaking removal of the shared-token CLI and server flow.
4. Prepare the VPS configuration with `VEXLO_HOSTED_MODE=true`, working dashboard and wildcard TLS, and operator credentials. Do not install the new binary with `VEXLO_HOSTED_MODE=false`: it will intentionally refuse to start.

## Tagging A Release

Create and push the release version as a semantic version tag:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

That triggers [.github/workflows/release.yml](.github/workflows/release.yml), which publishes:

- `vexlo-server-linux-amd64.tar.gz`
- `vexlo-server-linux-arm64.tar.gz`
- `vexlo-linux-amd64.tar.gz`
- `vexlo-linux-arm64.tar.gz`
- `vexlo-darwin-amd64.tar.gz`
- `vexlo-darwin-arm64.tar.gz`
- `vexlo-windows-amd64.zip`
- `vexlo-windows-arm64.zip`
- `SHA256SUMS.txt`

## Suggested GitHub Release Text

Title:

```text
Vexlo vX.Y.Z
```

Summary:

```text
Vexlo gives developers a temporary public HTTPS URL and private request-inspection dashboard with `vexlo http <port>`.
```

## Post-Release Checks

1. Open the GitHub release page.
2. Verify every archive uploaded successfully.
3. Verify `SHA256SUMS.txt` is present.
4. Download at least one archive and confirm it extracts cleanly.
5. If you are publishing deployment guidance, confirm the server artifact names in the docs still match the release assets.
6. Deploy the hosted server deliberately, verify `/healthz`, then test a real tunnel, its public URL, and private dashboard.
7. Verify `/install.sh` and `/install.ps1` return the release installers, then run each one-line command on a clean supported device and confirm `vexlo http 3000` works.
