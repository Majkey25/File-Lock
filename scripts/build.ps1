param([string]$Go = 'go')

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $targetOS = & $Go env GOOS
    $targetArch = & $Go env GOARCH
    if ($LASTEXITCODE -ne 0) { throw 'Cannot determine build target.' }
    $platform = switch ($targetOS) { 'windows' { 'windows' } 'linux' { 'linux' } 'darwin' { 'macos' } default { throw "Unsupported OS: $targetOS" } }
    $architecture = switch ($targetArch) { 'amd64' { 'x64' } 'arm64' { 'arm64' } default { throw "Unsupported architecture: $targetArch" } }
    $name = "File-Lock-$platform-$architecture"
    $package = Join-Path $root 'dist/package'
    if (Test-Path -LiteralPath $package) { throw 'dist/package already exists. Use a fresh build directory.' }
    New-Item -ItemType Directory -Path "$package/licenses" -Force | Out-Null
    $executable = if ($targetOS -eq 'windows') { 'FileLock.exe' } else { 'file-lock' }
    & $Go build -trimpath -ldflags='-s -w' -o "$package/$executable" .
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
    Copy-Item -LiteralPath README.md, LICENSE -Destination $package
    Copy-Item -LiteralPath docs -Destination "$package/docs" -Recurse
    if ($targetOS -eq 'windows') { Copy-Item -LiteralPath File-Lock.cmd -Destination $package }
    $goRoot = & $Go env GOROOT
    if ($LASTEXITCODE -ne 0) { throw 'Cannot locate Go license.' }
    Copy-Item -LiteralPath "$goRoot/LICENSE" -Destination "$package/licenses/go.txt"
    Copy-Item -LiteralPath "$goRoot/PATENTS" -Destination "$package/licenses/go-patents.txt"
    $modules = & $Go list -deps -f '{{with .Module}}{{.Path}}|{{.Dir}}{{end}}' .
    if ($LASTEXITCODE -ne 0) { throw 'Cannot collect dependency licenses.' }
    foreach ($module in ($modules | Where-Object { $_ } | Sort-Object -Unique)) {
        $parts = $module -split '\|', 2
        $license = Join-Path $parts[1] 'LICENSE'
        if ($parts[0] -ne 'github.com/Majkey25/File-Lock' -and (Test-Path -LiteralPath $license)) {
            $name = ($parts[0] -replace '[/\\]', '_') + '.txt'
            Copy-Item -LiteralPath $license -Destination "$package/licenses/$name"
        }
    }
    if ($targetOS -eq 'windows') {
        $archive = Join-Path $root "dist/$name.zip"
        Compress-Archive -Path "$package/*" -DestinationPath $archive
    } else {
        $archive = Join-Path $root "dist/$name.tar.gz"
        tar -czf $archive -C $package .
        if ($LASTEXITCODE -ne 0) { throw 'Packaging failed.' }
    }
    $hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $([System.IO.Path]::GetFileName($archive))" | Set-Content -LiteralPath "dist/$name.sha256" -Encoding ascii
    Write-Output "Built $archive"
} finally {
    Pop-Location
}
