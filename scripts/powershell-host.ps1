# Windows PowerShell 5 may inherit PS7-only PSModulePath from CI/pwsh parents.
# Resolve built-in modules from the actual current host first; process-local only.
if ($PSVersionTable.PSEdition -eq 'Desktop') {
  $env:PSModulePath = "$PSHOME\Modules;$env:PSModulePath"
}

function Get-PrismDesktopDirectory {
  # Desktop is the virtual shell namespace; DesktopDirectory is the physical
  # shortcut directory, including when Explorer has never initialized the user.
  return [Environment]::GetFolderPath([Environment+SpecialFolder]::DesktopDirectory,[Environment+SpecialFolderOption]::DoNotVerify)
}
