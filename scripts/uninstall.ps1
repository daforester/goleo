# Goleo local dev teardown — reverses scripts/setup.ps1
#
#   .\scripts\uninstall.ps1          # unlink global packages + remove build artifacts
#   .\scripts\uninstall.ps1 -Full    # also delete node_modules + dist (deep clean)
#
# It does NOT change your npm prefix (setup.ps1 set it, but other global installs
# may rely on it) — a note is printed at the end if it looks Goleo-specific.

param(
    [switch]$Full
)

Write-Host "=== Goleo Local Teardown ===" -ForegroundColor Cyan
Write-Host ""

$RepoRoot = Resolve-Path "$PSScriptRoot\.."

# 1. Remove the global @goleo packages. `npm rm -g` alone can silently no-op on a
#    corrupted/partial install (an empty @goleo/<pkg> dir, a leftover npm-link
#    symlink, or missing bin shims — e.g. after mixing `npm link` with
#    `npm install -g`), so we also force-remove the leftover dirs and the `goleo`
#    command shims directly. (Note: `npm rm` is a native command — its failure
#    sets $LASTEXITCODE but does NOT throw, so we check the code, not try/catch.)
Write-Host ">> Removing global @goleo packages..." -ForegroundColor Yellow
$globalRoot = (npm root -g)
$globalPrefix = (npm prefix -g)
foreach ($pkg in @("@goleo/cli", "@goleo/bridge")) {
    npm rm -g $pkg 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "   npm rm -g $pkg" -ForegroundColor Green
    } else {
        Write-Host "   npm rm -g $pkg failed — cleaning manually" -ForegroundColor DarkGray
    }
    $pkgDir = Join-Path $globalRoot ($pkg -replace '/', '\')
    if (Test-Path $pkgDir) {
        Remove-Item -Recurse -Force $pkgDir -ErrorAction SilentlyContinue
        Write-Host "   removed leftover $pkgDir" -ForegroundColor Green
    }
}
# The `goleo` command shims npm drops in the global prefix (goleo/.cmd/.ps1).
foreach ($shim in @("goleo", "goleo.cmd", "goleo.ps1")) {
    $p = Join-Path $globalPrefix $shim
    if (Test-Path $p) {
        Remove-Item -Force $p -ErrorAction SilentlyContinue
        Write-Host "   removed shim $shim" -ForegroundColor Green
    }
}

# 2. Remove any leftover global source copy that setup.ps1 wrote into the linked
#    @goleo/cli package (usually gone once the link above is removed).
try {
    $globalCliDir = Join-Path (npm root -g) "@goleo\cli\goleo"
    if (Test-Path $globalCliDir) {
        Remove-Item -Recurse -Force $globalCliDir -ErrorAction SilentlyContinue
        Write-Host "   removed global goleo source copy" -ForegroundColor Green
    }
} catch {}

# 3. Remove built binaries.
Write-Host ">> Removing built binaries..." -ForegroundColor Yellow
foreach ($bin in @(
    (Join-Path $RepoRoot "goleo.exe"),
    (Join-Path $RepoRoot "goleo"),
    (Join-Path $RepoRoot "cli\npm\bin\goleo.exe"),
    (Join-Path $RepoRoot "cli\npm\bin\goleo")
)) {
    if (Test-Path $bin) {
        Remove-Item -Force $bin -ErrorAction SilentlyContinue
        Write-Host "   removed $bin" -ForegroundColor Green
    }
}

# 4. Remove the bundled Go source (produced by cli/npm/copy-source.js).
Write-Host ">> Removing bundled Go source..." -ForegroundColor Yellow
$bundled = Join-Path $RepoRoot "cli\npm\goleo"
if (Test-Path $bundled) {
    Remove-Item -Recurse -Force $bundled -ErrorAction SilentlyContinue
    Write-Host "   removed cli/npm/goleo (bundled source + vendor)" -ForegroundColor Green
} else {
    Write-Host "   (nothing bundled)" -ForegroundColor DarkGray
}

# 5. Deep clean (-Full): node_modules + TypeScript dist across the workspace.
if ($Full) {
    Write-Host ">> Deep clean (node_modules + dist)..." -ForegroundColor Yellow
    $targets = @(
        "node_modules",
        "bridge\node_modules", "bridge\dist",
        "cli\npm\node_modules"
    )
    foreach ($t in $targets) {
        $p = Join-Path $RepoRoot $t
        if (Test-Path $p) {
            Remove-Item -Recurse -Force $p -ErrorAction SilentlyContinue
            Write-Host "   removed $t" -ForegroundColor Green
        }
    }
}

Write-Host ""
Write-Host "=== Teardown complete ===" -ForegroundColor Cyan
Write-Host ""

# Note about the npm prefix setup.ps1 set (left untouched on purpose).
$NpmPrefix = if ($env:GOLEO_NPM_PREFIX) { $env:GOLEO_NPM_PREFIX } else { Join-Path $env:APPDATA "npm" }
Write-Host "Your npm global prefix is still set to:" -ForegroundColor White
Write-Host "  $((npm config get prefix))" -ForegroundColor Green
Write-Host "setup.ps1 set this; it was left unchanged (other global installs may use it)." -ForegroundColor DarkGray
Write-Host "To reset it to the default:  npm config delete prefix --location=user" -ForegroundColor DarkGray
if (-not $Full) {
    Write-Host ""
    Write-Host "Run with -Full to also delete node_modules and dist." -ForegroundColor DarkGray
}
