param([string]$Go = 'go')

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $package = Join-Path $root 'dist/package'
    if (Test-Path -LiteralPath $package) { throw 'dist/package already exists. Use a fresh build directory.' }
    New-Item -ItemType Directory -Path "$package/licenses" -Force | Out-Null
    & $Go build -trimpath -ldflags='-s -w' -o "$package/FileLock.exe" .
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
    Copy-Item -LiteralPath File-Lock.cmd, README.md, LICENSE -Destination $package
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
    $archive = Join-Path $root 'dist/File-Lock-windows-x64.zip'
    Compress-Archive -Path "$package/*" -DestinationPath $archive
    $hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  File-Lock-windows-x64.zip" | Set-Content -LiteralPath dist/SHA256SUMS.txt -Encoding ascii
    Write-Output "Built $archive"
} finally {
    Pop-Location
}
