param([string]$Go = 'go', [string]$Destination = 'build/bin/GO-THIRD-PARTY-NOTICES.txt')
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
$root = Split-Path $PSScriptRoot -Parent
$encoding = [Text.UTF8Encoding]::new($false)
$lines = [Collections.Generic.List[string]]::new()
$lines.Add('Prism Browser - Go runtime component licenses')
$lines.Add('Generated from the exact Windows production dependency graph; CLI/test-only modules are excluded.')
$goRoot = (& $Go env GOROOT).Trim()
if ($LASTEXITCODE) { throw 'Cannot locate Go license.' }
$lines.Add("`n=== Go standard library / runtime ===`n")
$lines.Add([IO.File]::ReadAllText((Join-Path $goRoot 'LICENSE')))
$modules = & $Go list -deps -tags production -f '{{if .Module}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}' .
if ($LASTEXITCODE) { throw 'Cannot collect production modules.' }
$modules = $modules | Where-Object { $_ -and !($_.StartsWith('github.com/axgiroud312-byte/prism-local-browser|')) } | Sort-Object -Unique
foreach ($module in $modules) {
  $parts = $module -split '\|', 3
  $moduleRoot = $parts[2]
  $licenses = @(Get-ChildItem -LiteralPath $moduleRoot -File | Where-Object { $_.Name -match '^(LICENSE|LICENCE|COPYING|NOTICE|COPYRIGHT)(\b|[-.])' })
  # Subcomponent terms accompany the root module terms where the production graph contains them.
  if ($parts[0] -eq 'github.com/wailsapp/go-webview2') { $licenses += Get-Item -LiteralPath (Join-Path $moduleRoot 'webviewloader/LICENSE') }
  if (!$licenses.Count) { throw "Missing license for $($parts[0])@$($parts[1])." }
  foreach ($license in $licenses) {
    $relative = $license.FullName.Substring($moduleRoot.Length).TrimStart('\', '/')
    $lines.Add("`n=== $($parts[0])@$($parts[1]) / $relative ===`n")
    $lines.Add([IO.File]::ReadAllText($license.FullName))
  }
}
$target = Join-Path $root $Destination
[IO.Directory]::CreateDirectory((Split-Path $target -Parent)) | Out-Null
[IO.File]::WriteAllText($target, ($lines -join "`n"), $encoding)
Write-Output "Collected licenses for $($modules.Count) linked Go modules and the Go runtime."
