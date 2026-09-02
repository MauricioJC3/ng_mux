# ngmux installer for Windows — downloads the latest prebuilt binary from
# GitHub Releases and puts it on your user PATH.
#
#   irm https://raw.githubusercontent.com/MauricioJC3/ng_mux/main/install.ps1 | iex
#
# Environment overrides:
#   $env:NGMUX_INSTALL_DIR   where to put ngmux.exe  (default: %LOCALAPPDATA%\Programs\ngmux)
#   $env:NGMUX_VERSION       tag to install          (default: latest release)

$ErrorActionPreference = 'Stop'

$repo = 'MauricioJC3/ng_mux'
$installDir = if ($env:NGMUX_INSTALL_DIR) { $env:NGMUX_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\ngmux' }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
	'AMD64' { 'amd64' }
	'ARM64' { 'arm64' }
	default { throw "unsupported architecture: $($env:PROCESSOR_ARCHITECTURE)" }
}

$tag = $env:NGMUX_VERSION
if (-not $tag) {
	$rel = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
	$tag = $rel.tag_name
}
if (-not $tag) { throw "could not determine the latest release (is one published yet?)" }

$asset = "ngmux_windows_${arch}.exe"
$url   = "https://github.com/$repo/releases/download/$tag/$asset"

Write-Host "ngmux $tag  (windows/$arch)"
Write-Host "  -> $installDir\ngmux.exe"

New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$dest = Join-Path $installDir 'ngmux.exe'
$tmp  = "$dest.new"
Invoke-WebRequest -Uri $url -OutFile $tmp

# Best-effort checksum check.
try {
	$sums = (Invoke-WebRequest -Uri "https://github.com/$repo/releases/download/$tag/SHA256SUMS" -UseBasicParsing).Content
	$want = ($sums -split "`n" | ForEach-Object { $_ -replace '\*','' } |
		Where-Object { $_ -match "\s$([regex]::Escape($asset))$" } |
		ForEach-Object { ($_ -split '\s+')[0] })
	if ($want) {
		$got = (Get-FileHash -Algorithm SHA256 $tmp).Hash.ToLower()
		if ($got -ne $want.ToLower()) { throw "checksum mismatch for $asset" }
	}
} catch { if ($_.Exception.Message -like '*checksum mismatch*') { throw } }

Move-Item -Force $tmp $dest

# --- Put the install dir on the user PATH (idempotent, case/backslash tolerant) ---
function Test-DirOnPath([string]$dir, [string]$scope) {
	$cur = [Environment]::GetEnvironmentVariable('Path', $scope)
	if (-not $cur) { return $false }
	$want = $dir.TrimEnd('\').ToLowerInvariant()
	foreach ($e in ($cur -split ';')) {
		if ($e -and $e.TrimEnd('\').ToLowerInvariant() -eq $want) { return $true }
	}
	return $false
}

$onUserPath = Test-DirOnPath $installDir 'User'
if (-not $onUserPath) {
	$cur = [Environment]::GetEnvironmentVariable('Path', 'User')
	$new = if ($cur) { $cur.TrimEnd(';') + ';' + $installDir } else { $installDir }
	[Environment]::SetEnvironmentVariable('Path', $new, 'User')
	Write-Host ""
	Write-Host "Added $installDir to your user PATH."
} else {
	Write-Host ""
	Write-Host "$installDir is already on your user PATH."
}

# Make it usable in this session immediately.
if (($env:Path -split ';' | ForEach-Object { $_.TrimEnd('\').ToLowerInvariant() }) -notcontains $installDir.TrimEnd('\').ToLowerInvariant()) {
	$env:Path = "$env:Path;$installDir"
}

# Broadcast the environment change so processes started afterwards (new
# terminals, launched from Explorer/Start) pick it up without a sign-out.
try {
	if (-not ('Win32.Env' -as [type])) {
		Add-Type -Namespace Win32 -Name Env -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("user32.dll", SetLastError=true, CharSet=System.Runtime.InteropServices.CharSet.Auto)]
public static extern System.IntPtr SendMessageTimeout(System.IntPtr hWnd, uint Msg, System.UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out System.UIntPtr lpdwResult);
'@
	}
	$res = [System.UIntPtr]::Zero
	[void][Win32.Env]::SendMessageTimeout([System.IntPtr]0xffff, 0x1A, [System.UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$res)
} catch { }

# --- Tell the user plainly where things stand ---
Write-Host ""
$profileHijacksPath = $false
foreach ($pf in @($PROFILE.CurrentUserAllHosts, $PROFILE.CurrentUserCurrentHost)) {
	if ($pf -and (Test-Path $pf) -and (Select-String -Path $pf -Pattern '\$env:Path\s*=[^=]' -Quiet -ErrorAction SilentlyContinue)) {
		$profileHijacksPath = $true
	}
}
if ($profileHijacksPath) {
	Write-Host "Note: your PowerShell profile sets `$env:Path directly, which can hide ngmux."
	Write-Host "      Either change that line to append (`$env:Path += ...), or add this once:"
	Write-Host ""
	Write-Host "        'function ngmux { & `"$dest`" @args }' | Add-Content `$PROFILE"
	Write-Host ""
} else {
	Write-Host "Done. Open a NEW terminal and run:  ngmux"
	Write-Host "(this session too:  ngmux)"
}
Write-Host "Full path, always works:  $dest"
