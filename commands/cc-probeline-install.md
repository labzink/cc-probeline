---
description: Install or update the cc-probeline binary and wire the Claude Code status line, choosing the right channel for this OS
---

# Install cc-probeline: /cc-probeline-install

Installs (or updates) the **cc-probeline** binary and wires it into the Claude Code
status line. The plugin itself ships only commands and a status-line wizard — the
compiled binary is delivered through a package manager (Homebrew / Scoop) or a
signed install script. This command picks the right channel for the user's OS,
**shows the exact command, and runs it only after the user approves the native
Bash permission prompt.** Never install anything silently.

## Usage

```
/cc-probeline-install
```

No arguments.

---

## Step 0 — Detect current state

Run these and read the results:

```bash
command -v cc-probeline && cc-probeline --version
```

```bash
cc-probeline --check
```

Decide the mode:

- **Not found in PATH** → this is a fresh **install**. Go to Step 1.
- **Found, `--check` says installation OK** → already installed and wired. Tell the
  user it is already set up, show the version, and offer an **update** (Step 1 with
  the upgrade variant) or stop if they decline.
- **Found, but `--check` reports the status line is not wired** → binary is present,
  only wiring is missing. Skip to Step 3.

## Step 1 — Detect platform and package manager

```bash
uname -s; command -v brew; command -v scoop
```

- `Darwin` / `Linux` → prefer **Homebrew** if `brew` exists, otherwise the **install
  script** (Step 2c).
- Windows (no `uname`, or `MINGW`/`MSYS`/`CYGWIN`) → use **Scoop** (Step 2b).

## Step 2 — Install (or update) the binary

Pick exactly one channel. **Show the user the exact command first, then run it via
the Bash tool** — the Bash permission prompt is the user's explicit "yes, run this".
Do not chain extra commands into the same call; keep the install command alone so
the prompt is readable.

### 2a — Homebrew (macOS / Linux)

Install:

```bash
brew install --cask labzink/homebrew-tap/cc-probeline
```

Update:

```bash
brew upgrade --cask labzink/homebrew-tap/cc-probeline
```

After a Homebrew install/upgrade the binary is on PATH but the status line is **not**
wired yet → continue to Step 3.

### 2b — Scoop (Windows)

Add the bucket once, then install:

```bash
scoop bucket add labzink https://github.com/labzink/scoop-bucket
scoop install cc-probeline
```

Update:

```bash
scoop update cc-probeline
```

After Scoop install the status line is **not** wired yet → continue to Step 3.

### 2c — Install script (macOS / Linux, no Homebrew)

This script downloads the matching release asset, verifies its SHA-256 against the
published `checksums.txt`, installs the binary, **and wires the status line** in one
step:

```bash
curl -fsSL https://raw.githubusercontent.com/labzink/cc-probeline/main/scripts/install.sh | sh
```

The install script already wires the status line, so **skip Step 3** and go to Step 4.

## Step 3 — Wire the status line (Homebrew / Scoop paths only)

The binary can wire itself into `~/.claude/settings.json`:

```bash
cc-probeline install --merge-settings --binary-path "$(command -v cc-probeline)"
```

If the binary reports that a **different** status line is already configured, it
asks before overwriting and records a backup (restored on uninstall). Relay that to
the user and let them decide — do not force it.

## Step 4 — Verify and report

```bash
cc-probeline --check
```

Expect `installation OK` and exit code 0. Then tell the user:

- the installed version,
- that the new status line appears in **new** Claude Code sessions (or after the
  current status line refreshes),
- that `/cc-probeline-config` customises rows, colour, probes, and display mode,
- that `cc-probeline uninstall` removes the status line and restores the previous one.

If `--check` fails, report the exact error and stop — do not retry blindly.
