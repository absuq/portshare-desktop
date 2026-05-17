# portshare Release Checklist

## Local Verification

Run from the release branch:

```powershell
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + (Join-Path (Get-Location) '.superpowers\tools\go1.26.2\go\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
go test ./...
go vet ./...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File '.\scripts\build-windows.ps1'
Get-FileHash '.\.superpowers\tmp\portshare-direct.exe' -Algorithm SHA256
```

If `go` is not on the local PATH, add the portable toolchain first as shown above; the Go binary is under `.superpowers\tools\go1.26.2\go\bin`. If a local portable toolchain exists under `.superpowers\tools`, the build script uses it first. CI uses the PATH tools installed by `setup-go` and `setup-mingw`.

## GitHub Verification

- PR is open against `main`.
- CI check is green.
- PR description includes validation commands.
- Release notes include the exe SHA256.

## Manual Two-Machine Verification

- Both computers run the same release exe.
- Both computers are logged into the same Tailscale tailnet.
- Both computers grant UAC administrator permission at startup.
- Pairing succeeds with the same shared secret.
- Trusted peer appears in both applications.
- Deleting a trusted peer removes it from the UI and revokes the Windows firewall rules.
- A service bound to `0.0.0.0` is reachable through the remote Tailscale IP after authorization.
- A TCP service bound only to `127.0.0.1` is reachable after localhost bridge appears.
- Pausing localhost bridge stops loopback-only service access through the Tailscale IP.
- Closing `portshare` stops the `17890` control listener.
- `tailscale ping <peer-ip>` shows whether the path is `direct`, `DERP`, or `peer-relay`.
- Link guardian optimization does not apply a route when current direct latency is already low.

## Release

```powershell
gh release create <tag> '.superpowers\tmp\portshare-direct.exe#portshare-direct.exe' --target main --title 'portshare <tag>' --notes-file <release-notes-file>
```
