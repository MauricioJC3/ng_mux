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

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $installDir) {
	[Environment]::SetEnvironmentVariable('Path', "$userPath;$installDir", 'User')
	$env:Path = "$env:Path;$installDir"
	Write-Host ""
	Write-Host "Added $installDir to your user PATH. Open a new terminal, then run: ngmux"
} else {
	Write-Host ""
	Write-Host "Installed. Run: ngmux"
}
