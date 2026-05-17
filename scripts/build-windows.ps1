$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$env:PATH = (Join-Path $repoRoot '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'

$resourcePath = Join-Path $repoRoot 'cmd\portshare\rsrc_windows_amd64.syso'
if (Test-Path $resourcePath) {
  Remove-Item -LiteralPath $resourcePath -Force
}

Push-Location (Join-Path $repoRoot 'cmd\portshare')
try {
  & 'windres.exe' -O coff -F pe-x86-64 -i 'portshare.rc' -o 'rsrc_windows_amd64.syso'
} finally {
  Pop-Location
}

try {
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' build `
  -ldflags='-H windowsgui' `
  -o '.superpowers\tmp\portshare-direct.exe' `
  '.\cmd\portshare'
} finally {
  if (Test-Path $resourcePath) {
    Remove-Item -LiteralPath $resourcePath -Force
  }
}
