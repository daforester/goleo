# Goleo local dev setup — links all npm packages so you can test without publishing

Write-Host "=== Goleo Local Setup ===" -ForegroundColor Cyan
Write-Host ""

# 0. Point npm's global prefix at a user-owned directory so `npm link` installs
#    into the user context instead of a system-wide location (no admin required).
Write-Host ">> Configuring user-level npm prefix..." -ForegroundColor Yellow
$NpmPrefix = if ($env:GOLEO_NPM_PREFIX) { $env:GOLEO_NPM_PREFIX } else { Join-Path $env:APPDATA "npm" }
if (-not (Test-Path $NpmPrefix)) { New-Item -ItemType Directory -Path $NpmPrefix -Force | Out-Null }
npm config set prefix "$NpmPrefix" --location=user
Write-Host "   npm global prefix -> $NpmPrefix" -ForegroundColor Green

$NpmBin = $NpmPrefix
$pathEntries = $env:PATH -split ';'
if ($pathEntries -notcontains $NpmBin) {
    Write-Host "   Add this to your PATH so the global bins resolve:" -ForegroundColor Yellow
    Write-Host "   $NpmBin" -ForegroundColor Green
} else {
    Write-Host "   $NpmBin already on PATH" -ForegroundColor Green
}

# 1. Build the TypeScript packages
Write-Host ">> Building TypeScript packages..." -ForegroundColor Yellow
Push-Location "$PSScriptRoot\..\bridge"
npm install
npm run build
if ($LASTEXITCODE -ne 0) { Write-Host "bridge build failed" -ForegroundColor Red; exit 1 }
Write-Host "   @goleo/bridge built" -ForegroundColor Green
Pop-Location

Push-Location "$PSScriptRoot\..\create-goleo-app"
npm install
npm run build
if ($LASTEXITCODE -ne 0) { Write-Host "create-goleo-app build failed" -ForegroundColor Red; exit 1 }
Write-Host "   create-goleo-app built" -ForegroundColor Green
Pop-Location

# 2. Link packages globally
Write-Host ""
Write-Host ">> Linking packages globally..." -ForegroundColor Yellow

Push-Location "$PSScriptRoot\..\bridge"
npm link
Write-Host "   @goleo/bridge -> global" -ForegroundColor Green
Pop-Location

Push-Location "$PSScriptRoot\..\create-goleo-app"
npm link
Write-Host "   create-goleo-app -> global" -ForegroundColor Green
Pop-Location

# 3. Build the Go CLI binary
Write-Host ""
Write-Host ">> Building Go CLI binary..." -ForegroundColor Yellow
Push-Location "$PSScriptRoot\.."
go build -o goleo.exe .\cli\goleo\
if ($LASTEXITCODE -ne 0) { Write-Host "Go build failed" -ForegroundColor Red; exit 1 }
Write-Host "   goleo.exe built" -ForegroundColor Green

# Place it where @goleo/cli expects it
$cliBinDir = "$PSScriptRoot\..\cli\npm\bin"
if (-not (Test-Path $cliBinDir)) { New-Item -ItemType Directory -Path $cliBinDir -Force }
Copy-Item "$PSScriptRoot\..\goleo.exe" "$cliBinDir\goleo.exe" -Force
Write-Host "   goleo.exe copied to cli/npm/bin/" -ForegroundColor Green
Pop-Location

# 4. Bundle Go source in the npm package
Write-Host ""
Write-Host ">> Bundling Go source in npm package..." -ForegroundColor Yellow
Push-Location "$PSScriptRoot\..\cli\npm"
node copy-source.js
if ($LASTEXITCODE -ne 0) { Write-Host "copy-source.js failed" -ForegroundColor Red; exit 1 }
Write-Host "   Go source bundled" -ForegroundColor Green
Pop-Location

# 5. Link @goleo/cli
Push-Location "$PSScriptRoot\..\cli\npm"
npm link
Write-Host "   @goleo/cli -> global" -ForegroundColor Green

# Copy bundled Go source directly to the npm global location
$globalCliDir = "$(npm root -g)\@goleo\cli"
if (Test-Path "$globalCliDir") {
    $globalGoleoDir = "$globalCliDir\goleo"
    if (Test-Path $globalGoleoDir) { Remove-Item -Recurse -Force $globalGoleoDir -ErrorAction SilentlyContinue }
    New-Item -ItemType Directory -Force -Path $globalGoleoDir | Out-Null
    $repoRoot = "$PSScriptRoot\.."
    Copy-Item "$repoRoot\go.mod" "$globalGoleoDir\go.mod" -Force
    Copy-Item "$repoRoot\go.sum" "$globalGoleoDir\go.sum" -Force
    Copy-Item -Recurse "$repoRoot\runtime" "$globalGoleoDir\runtime" -Force
    Copy-Item -Recurse "$repoRoot\bridge" "$globalGoleoDir\bridge" -Force
    if (Test-Path "$repoRoot\vendor") {
        Copy-Item -Recurse "$repoRoot\vendor" "$globalGoleoDir\vendor" -Force
    }
    Write-Host "   goleo source (+ vendored deps) copied to global install" -ForegroundColor Green
} else {
    Write-Host "   Warning: @goleo/cli not found at npm global root - source not copied" -ForegroundColor Yellow
}
Pop-Location

# 6. Install root workspace deps
Write-Host ""
Write-Host ">> Installing workspace dependencies..." -ForegroundColor Yellow
Push-Location "$PSScriptRoot\.."
npm install
Pop-Location

Write-Host ""
Write-Host "=== Setup complete! ===" -ForegroundColor Cyan
Write-Host ""
Write-Host "Global packages were installed under $NpmPrefix (user context)." -ForegroundColor White
if ($pathEntries -notcontains $NpmBin) {
    Write-Host "Make sure $NpmBin is on your PATH before running the commands below." -ForegroundColor Yellow
}
Write-Host ""
Write-Host "Try these commands from anywhere:" -ForegroundColor White
Write-Host "  npx create-goleo-app my-test-app" -ForegroundColor Green
Write-Host "  npx goleo version" -ForegroundColor Green
Write-Host ""
Write-Host "In the scaffolded project (until published):" -ForegroundColor White
Write-Host "  cd my-test-app\frontend" -ForegroundColor Green
Write-Host "  npm link @goleo/bridge" -ForegroundColor Green
Write-Host "  npm install" -ForegroundColor Green
Write-Host "  cd .." -ForegroundColor Green
Write-Host "  npx goleo dev" -ForegroundColor Green
Write-Host "  npx goleo build" -ForegroundColor Green
