#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Build and install hopper, then wire shell integration.

Usage:
  scripts/install-common.sh --shell <zsh|bash> [--prefix DIR] [--bin-name NAME] [--no-shell]

Options:
  --shell NAME     Shell integration to wire (zsh or bash)
  --prefix DIR     Install directory (default: /usr/local/bin)
  --bin-name NAME  Installed binary name (default: hopper)
  --no-shell       Skip editing shell rc file
  -h, --help       Show this help
EOF
}

SHELL_KIND=""
PREFIX="/usr/local/bin"
BIN_NAME="hopper"
UPDATE_SHELL=1

while [[ $# -gt 0 ]]; do
  case "$1" in
    --shell)
      SHELL_KIND="${2:-}"
      shift 2
      ;;
    --prefix)
      PREFIX="${2:-}"
      shift 2
      ;;
    --bin-name)
      BIN_NAME="${2:-}"
      shift 2
      ;;
    --no-shell)
      UPDATE_SHELL=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ "$SHELL_KIND" != "zsh" && "$SHELL_KIND" != "bash" ]]; then
  echo "--shell must be zsh or bash" >&2
  usage >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "go is required but not found in PATH" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

echo "Building hopper..."
go build -o hopper .

DEST_DIR="$(cd "$PREFIX" 2>/dev/null && pwd || true)"
if [[ -z "$DEST_DIR" ]]; then
  DEST_DIR="$PREFIX"
fi
DEST_PATH="$DEST_DIR/$BIN_NAME"

echo "Installing ${BIN_NAME} to ${DEST_PATH}..."
mkdir -p "$DEST_DIR" 2>/dev/null || true
if [[ -w "$DEST_DIR" ]]; then
  install -m 755 hopper "$DEST_PATH"
else
  sudo install -m 755 hopper "$DEST_PATH"
fi

if [[ "$BIN_NAME" == "hopper" ]]; then
  LEGACY_PATH="$DEST_DIR/hydra"
  if [[ -e "$LEGACY_PATH" ]]; then
    echo "Removing legacy binary ${LEGACY_PATH}..."
    if [[ -w "$DEST_DIR" ]]; then
      rm -f "$LEGACY_PATH"
    else
      sudo rm -f "$LEGACY_PATH"
    fi
  fi
fi

if [[ "$UPDATE_SHELL" -eq 1 ]]; then
  START_MARKER="# >>> hopper >>>"
  END_MARKER="# <<< hopper <<<"
  BLOCK="$($DEST_PATH init "$SHELL_KIND")"

  if [[ "$SHELL_KIND" == "zsh" ]]; then
    RC_FILE="${ZDOTDIR:-$HOME}/.zshrc"
  else
    RC_FILE="$HOME/.bashrc"
  fi

  touch "$RC_FILE"
  TMP_FILE="$(mktemp)"

  awk -v start="$START_MARKER" -v end="$END_MARKER"  '
    BEGIN { in_block=0 }
    $0 == start { in_block=1; next }
    $0 == end { in_block=0; next }
    !in_block { print }
  ' "$RC_FILE" > "$TMP_FILE"

  if [[ -s "$TMP_FILE" ]]; then
    printf "\n" >> "$TMP_FILE"
  fi

  {
    printf "%s\n" "$START_MARKER"
    printf "%s\n" "$BLOCK"
    printf "%s\n" "$END_MARKER"
  } >> "$TMP_FILE"

  mv "$TMP_FILE" "$RC_FILE"

  echo "Updated ${RC_FILE}."
  echo "Run: source ${RC_FILE}"
fi

echo "Done."
