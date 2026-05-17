$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$portableMingwBin = Join-Path $repoRoot '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin'
if (Test-Path (Join-Path $portableMingwBin 'windres.exe')) {
  $env:PATH = $portableMingwBin + ';' + $env:PATH
}

$windres = Get-Command 'windres.exe' -ErrorAction SilentlyContinue
if (-not $windres) {
  throw "windres.exe was not found. Install MinGW and ensure windres.exe is on PATH, or place w64devkit at .superpowers\tools\w64devkit-1.23.0."
}

$portableGo = Join-Path $repoRoot '.superpowers\tools\go1.26.2\go\bin\go.exe'
if (Test-Path $portableGo) {
  $go = $portableGo
} else {
  $goCommand = Get-Command 'go.exe' -ErrorAction SilentlyContinue
  if (-not $goCommand) {
    $goCommand = Get-Command 'go' -ErrorAction SilentlyContinue
  }
  if (-not $goCommand) {
    throw "go was not found. Install Go and ensure go is on PATH, or place portable Go at .superpowers\tools\go1.26.2."
  }
  $go = $goCommand.Source
}

$env:CGO_ENABLED = '1'

$outputDir = Join-Path $repoRoot '.superpowers\tmp'
New-Item -ItemType Directory -Path $outputDir -Force | Out-Null

$resourcePath = Join-Path $repoRoot 'cmd\portshare\rsrc_windows_amd64.syso'
if (Test-Path $resourcePath) {
  Remove-Item -LiteralPath $resourcePath -Force
}

Push-Location (Join-Path $repoRoot 'cmd\portshare')
try {
  & $windres.Source -O coff -F pe-x86-64 -i 'portshare.rc' -o 'rsrc_windows_amd64.syso'
} finally {
  Pop-Location
}

try {
& $go build `
  -ldflags='-H windowsgui' `
  -o '.superpowers\tmp\portshare-direct.exe' `
  '.\cmd\portshare'
} finally {
  if (Test-Path $resourcePath) {
    Remove-Item -LiteralPath $resourcePath -Force
  }
}
