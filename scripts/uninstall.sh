#!/usr/bin/env bash
# Goleo local dev teardown — reverses scripts/setup.sh
#
#   ./scripts/uninstall.sh          # unlink global packages + remove build artifacts
#   ./scripts/uninstall.sh --full   # also delete node_modules + dist (deep clean)
#
# It does NOT change your npm prefix (setup.sh set it, but other global installs
# may rely on it) — a note is printed at the end.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m'

info() { echo -e "${CYAN}$1${NC}"; }
step() { echo -e "\n${YELLOW}>> $1${NC}"; }
ok()   { echo -e "   ${GREEN}$1${NC}"; }
skip() { echo -e "   ${GRAY}$1${NC}"; }

FULL=0
if [ "${1:-}" = "--full" ] || [ "${1:-}" = "-f" ]; then
  FULL=1
fi

info "=== Goleo Local Teardown ==="

# 1. Unlink the globally linked packages (best-effort — ignore if not present).
step "Unlinking global packages..."
for pkg in "@goleo/cli" "@goleo/bridge"; do
  if npm rm -g "$pkg" >/dev/null 2>&1; then
    ok "removed global link: $pkg"
  else
    skip "(not linked: $pkg)"
  fi
done

# 2. Remove any leftover global source copy setup.sh wrote into the linked
#    @goleo/cli package (usually gone once the link above is removed).
GLOBAL_GOLEO="$(npm root -g 2>/dev/null)/@goleo/cli/goleo"
if [ -d "$GLOBAL_GOLEO" ]; then
  rm -rf "$GLOBAL_GOLEO"
  ok "removed global goleo source copy"
fi

# 3. Remove built binaries.
step "Removing built binaries..."
for bin in \
  "$ROOT/goleo" "$ROOT/goleo.exe" \
  "$ROOT/cli/npm/bin/goleo" "$ROOT/cli/npm/bin/goleo.exe"; do
  if [ -f "$bin" ]; then
    rm -f "$bin"
    ok "removed $bin"
  fi
done

# 4. Remove the bundled Go source (produced by cli/npm/copy-source.js).
step "Removing bundled Go source..."
if [ -d "$ROOT/cli/npm/goleo" ]; then
  rm -rf "$ROOT/cli/npm/goleo"
  ok "removed cli/npm/goleo (bundled source + vendor)"
else
  skip "(nothing bundled)"
fi

# 5. Deep clean (--full): node_modules + TypeScript dist across the workspace.
if [ "$FULL" -eq 1 ]; then
  step "Deep clean (node_modules + dist)..."
  for t in \
    "node_modules" \
    "bridge/node_modules" "bridge/dist" \
    "create-goleo-app/node_modules" "create-goleo-app/dist" \
    "cli/npm/node_modules" \
    "frontend/node_modules"; do
    if [ -e "$ROOT/$t" ]; then
      rm -rf "${ROOT:?}/$t"
      ok "removed $t"
    fi
  done
fi

echo ""
info "=== Teardown complete ==="
echo ""

# Note about the npm prefix setup.sh set (left untouched on purpose).
echo -e "Your npm global prefix is still set to:"
echo -e "  ${GREEN}$(npm config get prefix 2>/dev/null)${NC}"
echo -e "${GRAY}setup.sh set this; it was left unchanged (other global installs may use it).${NC}"
echo -e "${GRAY}To reset it to the default:  npm config delete prefix --location=user${NC}"
if [ "$FULL" -eq 0 ]; then
  echo ""
  echo -e "${GRAY}Run with --full to also delete node_modules and dist.${NC}"
fi
