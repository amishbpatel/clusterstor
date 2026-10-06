$ErrorActionPreference = "Stop"

$repo = Split-Path -Parent $PSScriptRoot
$outDir = Join-Path $repo "dist\windows"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

Write-Host "Building ClusterStor Windows agent..."
Push-Location $repo
try {
  $env:GOOS = "windows"
  $env:GOARCH = "amd64"
  go build -trimpath -o (Join-Path $outDir "clusterstor-agent.exe") ./cmd/agent
} finally {
  Pop-Location
  Remove-Item Env:GOOS -ErrorAction SilentlyContinue
  Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
}

Write-Host "Built: $outDir\clusterstor-agent.exe"
