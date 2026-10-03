# Release Guide

Vexlo has been hosted-only since v0.2.0. This guide covers subsequent releases;
older published releases retain their historical behavior. Do not treat a
successful build as proof that all live-service checks have passed.

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

3. Run `sh scripts/test_install.sh` on Linux or macOS. CI also runs the Windows
   installer fixture test in PowerShell 7 and Windows PowerShell 5.1 on a
   disposable runner. Do not run that test on your everyday Windows account:
   it temporarily updates the user PATH.
4. Read [README.md](README.md) and [deploy/README.md](deploy/README.md) once
   as a user, not as the author.
5. Confirm CI, including installer jobs, passes. For a feature or
   public-readiness claim, complete the relevant on-device and operational
   checks in [the hosted-service plan](docs/hosted-service-plan.md); CI
   fixture tests alone do not prove them.
6. Update [CHANGELOG.md](CHANGELOG.md) for the release.
7. Prepare any required VPS configuration changes, including TLS and operator
   credentials. The server fails closed if hosted mode is disabled; do not
   expect a public listener with `VEXLO_HOSTED_MODE=false`.

## Tagging A Release

Create and push the release version as a semantic version tag:

```bash
git tag -a vX.Y.Z -m "Vexlo vX.Y.Z"
git remote -v
git push master vX.Y.Z
```

This checkout uses `master` as its remote name. If `git remote -v` shows a
different name in your clone, use that name instead.

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
