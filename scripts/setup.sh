#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info()  { echo -e "${CYAN}$1${NC}"; }
step()  { echo -e "\n${YELLOW}>> $1${NC}"; }
ok()    { echo -e "   ${GREEN}$1${NC}"; }
fail()  { echo -e "   ${RED}$1${NC}"; exit 1; }

info "=== Goleo Local Setup ==="

# 1. Build the TypeScript packages
step "Building TypeScript packages..."

pushd "$ROOT/bridge" > /dev/null
npm install --silent
npm run build || fail "bridge build failed"
ok "@goleo/bridge built"
popd > /dev/null

pushd "$ROOT/create-goleo-app" > /dev/null
npm install --silent
npm run build || fail "create-goleo-app build failed"
ok "create-goleo-app built"
popd > /dev/null

# 2. Link packages globally
step "Linking packages globally..."

pushd "$ROOT/bridge" > /dev/null
npm link
ok "@goleo/bridge -> global"
popd > /dev/null

pushd "$ROOT/create-goleo-app" > /dev/null
npm link
ok "create-goleo-app -> global"
popd > /dev/null

# 3. Build the Go CLI binary
step "Building Go CLI binary..."

pushd "$ROOT" > /dev/null
go build -o goleo ./cli/ || fail "Go build failed"
ok "goleo binary built"

mkdir -p "$ROOT/cli/npm/bin"
cp goleo "$ROOT/cli/npm/bin/goleo"
ok "goleo binary copied to cli/npm/bin/"
popd > /dev/null

# 4. Link @goleo/cli
pushd "$ROOT/cli/npm" > /dev/null
npm link
ok "@goleo/cli -> global"
popd > /dev/null

# 5. Install root workspace deps
step "Installing workspace dependencies..."
pushd "$ROOT" > /dev/null
npm install --silent
popd > /dev/null

echo ""
info "=== Setup complete! ==="
echo ""
echo -e "Try these commands from anywhere:"
echo -e "  ${GREEN}npx create-goleo-app my-test-app${NC}"
echo -e "  ${GREEN}npx goleo version${NC}"
echo ""
echo -e "In the scaffolded project (until published):"
echo -e "  ${GREEN}cd my-test-app/frontend${NC}"
echo -e "  ${GREEN}npm link @goleo/bridge${NC}"
echo -e "  ${GREEN}npm install${NC}"
echo -e "  ${GREEN}cd ..${NC}"
echo -e "  ${GREEN}npx goleo dev${NC}"
echo -e "  ${GREEN}npx goleo build${NC}"
