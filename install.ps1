# tmux2 installer for Windows — downloads the latest prebuilt binary from
# GitHub Releases and puts it on your user PATH.
#
#   irm https://raw.githubusercontent.com/MauricioJC3/ng_mux/main/install.ps1 | iex
#
# Environment overrides:
#   $env:TMUX2_INSTALL_DIR   where to put tmux2.exe  (default: %LOCALAPPDATA%\Programs\tmux2)
#   $env:TMUX2_VERSION       tag to install          (default: latest release)

$ErrorActionPreference = 'Stop'

$repo = 'MauricioJC3/ng_mux'
$installDir = if ($env:TMUX2_INSTALL_DIR) { $env:TMUX2_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\tmux2' }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
	'AMD64' { 'amd64' }
	'ARM64' { 'arm64' }
	default { throw "unsupported architecture: $($env:PROCESSOR_ARCHITECTURE)" }
}

$tag = $env:TMUX2_VERSION
if (-not $tag) {
	$rel = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
	$tag = $rel.tag_name
}
if (-not $tag) { throw "could not determine the latest release (is one published yet?)" }

$asset = "tmux2_windows_${arch}.exe"
$url   = "https://github.com/$repo/releases/download/$tag/$asset"

Write-Host "tmux2 $tag  (windows/$arch)"
Write-Host "  -> $installDir\tmux2.exe"

New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$dest = Join-Path $installDir 'tmux2.exe'
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
	Write-Host "Added $installDir to your user PATH. Open a new terminal, then run: tmux2"
} else {
	Write-Host ""
	Write-Host "Installed. Run: tmux2"
}
