# Windows PowerShell 5 may inherit PS7-only PSModulePath from CI/pwsh parents.
# Resolve built-in modules from the actual current host first; process-local only.
if ($PSVersionTable.PSEdition -eq 'Desktop') {
  $env:PSModulePath = "$PSHOME\Modules;$env:PSModulePath"
}
