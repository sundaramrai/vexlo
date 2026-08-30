# Release Guide

This repo is released as a self-hosted tunnel server plus client binaries.

## Scope

- `vexlo-server`
  Public/self-hosted server binary
- `vexlo`
  Tunnel client binary

## Before Tagging

1. Confirm the working tree is in the state you want to publish.
2. Run local verification:

```bash
go test ./...
go build ./...
```

1. Read [README.md](README.md) and [deploy/README.md](deploy/README.md) once as a user, not as the author.
2. Update [CHANGELOG.md](CHANGELOG.md) if the release contents changed.

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
- `SHA256SUMS.txt`

## Suggested GitHub Release Text

Title:

```text
Vexlo vX.Y.Z
```

Summary:

```text
Vexlo is a self-hosted localhost tunnel server with a terminal-style dashboard for request capture, replay, and mutation.
```

## Post-Release Checks

1. Open the GitHub release page.
2. Verify every archive uploaded successfully.
3. Verify `SHA256SUMS.txt` is present.
4. Download at least one archive and confirm it extracts cleanly.
5. If you are publishing deployment guidance, confirm the server artifact names in the docs still match the release assets.
