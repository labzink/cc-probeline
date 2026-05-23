#!/usr/bin/env bash
# install.sh — install cc-probeline binary and wire Claude Code statusLine.
#
# Usage:
#   install.sh [--dest DIR] [--no-settings] [--force]
#              [--refresh-interval N] [--verbose] [--help|-h]
#
# Exit codes:
#   0   success
#   1   binary copy / verify / settings failure
#   64  unknown flag
set -euo pipefail

tmp=""
cleanup() {
    if [ -n "$tmp" ]; then rm -f "$tmp"; fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# print_usage — written before flag parsing so --help can reference it early.
# ---------------------------------------------------------------------------
print_usage() {
    cat <<'EOF'
Usage: install.sh [OPTIONS]

Install cc-probeline and wire the Claude Code statusLine.

Options:
  --dest DIR            Install binary to DIR (default: ~/.local/bin/cc-probeline).
                        Can also be set via CC_PROBELINE_INSTALL_DEST env var.
  --no-settings         Copy binary only; skip statusLine merge in settings.json.
  --force               Overwrite a foreign statusLine (backup is created).
  --refresh-interval N  Set statusLine refresh interval in seconds (default: 5).
  --verbose             Enable bash -x trace output.
  --help, -h            Print this help and exit.

Environment:
  CC_PROBELINE_INSTALL_DEST   Override default install destination.

Examples:
  install.sh
  install.sh --dest /usr/local/bin/cc-probeline
  install.sh --force --refresh-interval 3
  install.sh --no-settings
EOF
}

# ---------------------------------------------------------------------------
# Step 1: detect OS + arch.
# ---------------------------------------------------------------------------
os=$(uname -s)
arch=$(uname -m)

case "$os-$arch" in
    Darwin-arm64|Darwin-x86_64|Linux-x86_64|Linux-aarch64) ;;
    *) echo "Unsupported: $os-$arch"; exit 1 ;;
esac

# ---------------------------------------------------------------------------
# Step 2: locate binary next to the script (parent dir = project root).
# ---------------------------------------------------------------------------
self_dir=$(cd "$(dirname "$0")" && pwd)
proj_dir=$(cd "$self_dir/.." && pwd)

src="$proj_dir/cc-probeline"
if [ ! -x "$src" ]; then
    src="$proj_dir/cc-probeline-$os-$arch"
fi
if [ ! -x "$src" ]; then
    echo "Binary not found near $self_dir; build with: go build -o cc-probeline ./cmd/cc-probeline/" >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Step 3: parse flags.
# ---------------------------------------------------------------------------
no_settings=""
force=""
rinterval=""
dest="${CC_PROBELINE_INSTALL_DEST:-$HOME/.local/bin/cc-probeline}"

while [ $# -gt 0 ]; do
    case "$1" in
        --dest)
            if [ $# -lt 2 ]; then
                echo "install.sh: --dest requires an argument" >&2
                exit 64
            fi
            dest="$2"
            shift 2
            ;;
        --no-settings)
            no_settings=1
            shift
            ;;
        --force)
            force=1
            shift
            ;;
        --refresh-interval)
            if [ $# -lt 2 ]; then
                echo "install.sh: --refresh-interval requires an argument" >&2
                exit 64
            fi
            rinterval="$2"
            shift 2
            ;;
        --verbose)
            set -x
            shift
            ;;
        --help|-h)
            print_usage
            exit 0
            ;;
        *)
            echo "Unknown flag: $1" >&2
            exit 64
            ;;
    esac
done

# ---------------------------------------------------------------------------
# Step 4: mkdir + atomic copy.
# ---------------------------------------------------------------------------
dest_dir=$(dirname "$dest")
mkdir -p "$dest_dir"

tmp="$dest.tmp.$$"
cp "$src" "$tmp"
chmod +x "$tmp"
mv "$tmp" "$dest"

# ---------------------------------------------------------------------------
# Step 5: verify the installed binary is runnable.
# ---------------------------------------------------------------------------
if ! "$dest" --version >/dev/null 2>&1; then
    echo "install.sh: installed binary verification failed: $dest --version returned non-zero" >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Step 6: PATH warning (non-fatal).
# ---------------------------------------------------------------------------
case ":${PATH}:" in
    *":${dest_dir}:"*) ;;
    *)
        echo "Warning: ${dest_dir} is not in PATH."
        echo "  To use cc-probeline from any shell, add the following to your profile:"
        echo "    export PATH=\"${dest_dir}:\$PATH\""
        echo "  (install.sh: not in PATH)"
        ;;
esac

# ---------------------------------------------------------------------------
# Step 7: merge statusLine into settings.json (unless --no-settings).
# ---------------------------------------------------------------------------
if [ -z "$no_settings" ]; then
    args=("install" "--merge-settings" "--binary-path" "$dest")
    if [ -n "$force" ]; then
        args+=("--force")
    fi
    if [ -n "$rinterval" ]; then
        args+=("--refresh-interval" "$rinterval")
    fi

    "$dest" "${args[@]}" || exit $?
fi

# ---------------------------------------------------------------------------
# Step 8: smoke check — pipe minimal JSON payload, expect exit 0.
# ---------------------------------------------------------------------------
smoke_payload='{"transcript_path":"/dev/null","session_id":"smoke-12345678","model":{"id":"claude-sonnet-4-5"},"cwd":"/tmp","effort":{"level":"medium"},"context_window":{"context_window_size":200000,"current_usage":{}}}'
if printf '%s' "$smoke_payload" | "$dest" >/dev/null 2>&1; then
    echo "cc-probeline: installed at $dest"
else
    echo "install.sh: smoke check failed (binary ran but returned non-zero)" >&2
    exit 1
fi
