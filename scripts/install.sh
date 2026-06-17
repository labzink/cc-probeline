#!/bin/sh
# install.sh — install cc-probeline binary and wire Claude Code statusLine.
# POSIX sh (the documented invocation is `curl ... | sh`; on Debian/Ubuntu that
# is dash, which has no arrays — keep this script array-free).
#
# Usage:
#   install.sh [--dest DIR] [--no-settings] [--force]
#              [--refresh-interval N] [--verbose] [--help|-h]
#
# Exit codes:
#   0   success
#   1   binary copy / verify / settings failure
#   64  unknown flag
set -eu  # POSIX sh has no `pipefail`; pipes that matter guard with `|| true`.

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
# download_release — when no local binary is found, fetch the release archive
# for the current OS/arch from GitHub Releases, verify its SHA256 against
# checksums.txt, extract the binary, and echo the path to the extracted binary.
# Honours CC_PROBELINE_VERSION (e.g. "0.1.0") to pin a version; otherwise the
# latest release is used. Diagnostics go to stderr; only the binary path is
# written to stdout so callers can capture it.
# ---------------------------------------------------------------------------
REPO="labzink/cc-probeline"

# resolve_version — echo the target version (no leading "v"). Honors
# CC_PROBELINE_VERSION; otherwise resolves the latest release tag from GitHub.
# Returns 1 (echoing nothing) when the latest tag can't be resolved (e.g. offline).
resolve_version() {
    local v="${CC_PROBELINE_VERSION:-}"
    if [ -n "$v" ]; then
        echo "${v#v}"
        return 0
    fi
    v=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
        | grep '"tag_name"' | head -1 \
        | sed -E 's/.*"tag_name":[[:space:]]*"v?([^"]+)".*/\1/' || true)
    [ -n "$v" ] || return 1
    echo "$v"
}

download_release() {
    local rel_os="$1" rel_arch="$2"
    local goos goarch
    case "$rel_os" in
        Darwin) goos=darwin ;;
        Linux)  goos=linux ;;
        *) echo "download_release: unsupported OS: $rel_os" >&2; return 1 ;;
    esac
    case "$rel_arch" in
        x86_64|amd64)  goarch=amd64 ;;
        arm64|aarch64) goarch=arm64 ;;
        *) echo "download_release: unsupported arch: $rel_arch" >&2; return 1 ;;
    esac

    local ver base
    ver=$(resolve_version || true)
    if [ -z "$ver" ]; then
        echo "download_release: could not resolve the latest release version" >&2
        return 1
    fi
    if [ -n "${CC_PROBELINE_VERSION:-}" ]; then
        base="https://github.com/${REPO}/releases/download/v${ver}"
    else
        base="https://github.com/${REPO}/releases/latest/download"
    fi

    local asset="cc-probeline_${ver}_${goos}_${goarch}.tar.gz"
    local dldir
    dldir=$(mktemp -d)

    if ! curl -fsSL -o "${dldir}/${asset}" "${base}/${asset}"; then
        echo "download_release: failed to download ${asset} from ${base}" >&2
        return 1
    fi
    if ! curl -fsSL -o "${dldir}/checksums.txt" "${base}/checksums.txt"; then
        echo "download_release: failed to download checksums.txt from ${base}" >&2
        return 1
    fi

    # Verify SHA256 against checksums.txt (goreleaser format: "<sha256>  <file>").
    local want got
    want=$(grep " ${asset}\$" "${dldir}/checksums.txt" | awk '{print $1}' | head -1 || true)
    if [ -z "$want" ]; then
        echo "download_release: ${asset} not listed in checksums.txt" >&2
        return 1
    fi
    if command -v sha256sum >/dev/null 2>&1; then
        got=$(sha256sum "${dldir}/${asset}" | awk '{print $1}')
    else
        got=$(shasum -a 256 "${dldir}/${asset}" | awk '{print $1}')
    fi
    if [ "$want" != "$got" ]; then
        echo "download_release: SHA256 mismatch for ${asset}" >&2
        echo "  expected: $want" >&2
        echo "  actual:   $got" >&2
        return 1
    fi

    if ! tar -xzf "${dldir}/${asset}" -C "${dldir}"; then
        echo "download_release: failed to extract ${asset}" >&2
        return 1
    fi
    if [ ! -x "${dldir}/cc-probeline" ]; then
        echo "download_release: cc-probeline binary not found inside ${asset}" >&2
        return 1
    fi
    echo "${dldir}/cc-probeline"
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
# Step 2: parse flags.
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
# Step 3: already up to date? When the binary at the target path matches the
# target release (and --force was not passed), there is nothing to download or
# re-wire — report and exit. An unresolved version (offline) falls through to a
# normal install rather than blocking. A "dev" build never equals a real
# release version, so it is always reinstalled.
# ---------------------------------------------------------------------------
if [ -z "$force" ] && [ -x "$dest" ]; then
    installed_ver=$("$dest" --version 2>/dev/null | head -1 \
        | sed -E 's/^cc-probeline ([^ ]+).*/\1/' || true)
    target_ver=$(resolve_version || true)
    if [ -n "$target_ver" ] && [ "$installed_ver" = "$target_ver" ]; then
        echo "cc-probeline $installed_ver is already the latest version — nothing to do."
        echo "To reinstall the same version anyway, pass --force:"
        echo "  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install.sh | sh -s -- --force"
        exit 0
    fi
fi

# ---------------------------------------------------------------------------
# Step 4: locate binary next to the script (parent dir = project root), or
# download the release asset from GitHub when none is present.
# ---------------------------------------------------------------------------
self_dir=$(cd "$(dirname "$0")" && pwd)
proj_dir=$(cd "$self_dir/.." && pwd)

src="$proj_dir/cc-probeline"
if [ ! -f "$src" ] || [ ! -x "$src" ]; then
    src="$proj_dir/cc-probeline-$os-$arch"
fi
if [ ! -f "$src" ] || [ ! -x "$src" ]; then
    # Normal path for `curl … | sh`: no binary sits next to the piped script,
    # so fetch the release asset from GitHub and verify its checksum. (Only a
    # local checkout with a pre-built binary skips this.)
    echo "Downloading cc-probeline from GitHub Releases..." >&2
    src=$(download_release "$os" "$arch") || {
        echo "Could not download a cc-probeline release from GitHub." >&2
        echo "Building from source: go build -o cc-probeline ./cmd/cc-probeline/" >&2
        exit 1
    }
fi

# ---------------------------------------------------------------------------
# Step 5: mkdir + atomic copy.
# ---------------------------------------------------------------------------
dest_dir=$(dirname "$dest")
mkdir -p "$dest_dir"

tmp="$dest.tmp.$$"
cp "$src" "$tmp"
chmod +x "$tmp"
mv "$tmp" "$dest"

# ---------------------------------------------------------------------------
# Step 6: verify the installed binary is runnable.
# ---------------------------------------------------------------------------
if ! "$dest" --version >/dev/null 2>&1; then
    echo "install.sh: installed binary verification failed: $dest --version returned non-zero" >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Step 7: PATH warning (non-fatal).
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
# Step 8: merge statusLine into settings.json (unless --no-settings).
# ---------------------------------------------------------------------------
if [ -z "$no_settings" ]; then
    # POSIX arg accumulation via positional params (no bash arrays). Safe here:
    # the script's own args were parsed earlier and "$@" is unused past this point.
    set -- install --merge-settings --binary-path "$dest"
    if [ -n "$force" ]; then
        set -- "$@" --force
    fi
    if [ -n "$rinterval" ]; then
        set -- "$@" --refresh-interval "$rinterval"
    fi

    "$dest" "$@" || exit $?
fi

# ---------------------------------------------------------------------------
# Step 9: smoke check — pipe minimal JSON payload, expect exit 0.
# ---------------------------------------------------------------------------
smoke_payload='{"transcript_path":"/dev/null","session_id":"smoke-12345678","model":{"id":"claude-sonnet-4-5"},"cwd":"/tmp","effort":{"level":"medium"},"context_window":{"context_window_size":200000,"current_usage":{}}}'
if printf '%s' "$smoke_payload" | "$dest" >/dev/null 2>&1; then
    echo "cc-probeline: installed at $dest"
else
    echo "install.sh: smoke check failed (binary ran but returned non-zero)" >&2
    exit 1
fi
