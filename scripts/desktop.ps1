param([ValidateSet('test','build','dev','doctor')][string]$Action = 'test')
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
$root = Split-Path $PSScriptRoot -Parent
$localGo = Join-Path $root '.tools/go/bin/go.exe'
if (Test-Path $localGo) { $go = $localGo } else {
  $command = Get-Command go -ErrorAction SilentlyContinue
  if (!$command) { throw 'Go is required. Install the version pinned in go.mod or use .tools/go.' }
  $go = $command.Source
}
$env:PATH = "$(Split-Path $go);$(Join-Path $root '.tools/bin');$env:PATH"
$env:GOPATH = Join-Path $root '.tools/gopath'
$env:GOCACHE = Join-Path $root '.tools/gocache'
$env:GOBIN = Join-Path $root '.tools/bin'
$env:GOTOOLCHAIN = 'local'
$env:CGO_ENABLED = '0'
Push-Location $root
try {
  switch ($Action) {
    'test' {
      if (!(Test-Path 'dist/index.html')) {
        & npm run build:desktop
        if ($LASTEXITCODE) { throw 'Embedded frontend build failed.' }
      }
      & $go test ./...
      if ($LASTEXITCODE) { throw 'Go tests failed.' }
      & $go vet ./...
      if ($LASTEXITCODE) { throw 'Go vet failed.' }
    }
    'build' {
      & wails build -clean -platform windows/amd64
      if ($LASTEXITCODE) { throw 'Windows build failed.' }
      & (Join-Path $PSScriptRoot 'collect-go-notices.ps1') -Go $go
      Copy-Item 'LICENSE','THIRD_PARTY_NOTICES.md' -Destination 'build/bin'
    }
    'dev' { & wails dev; if ($LASTEXITCODE) { throw 'Desktop dev failed.' } }
    'doctor' { & wails doctor; if ($LASTEXITCODE) { throw 'Wails doctor failed.' } }
  }
} finally { Pop-Location }
