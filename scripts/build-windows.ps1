$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$env:PATH = (Join-Path $repoRoot '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'

& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' build `
  -ldflags='-H windowsgui' `
  -o '.superpowers\tmp\portshare-direct.exe' `
  '.\cmd\portshare'
