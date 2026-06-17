---
description: Update cc-probeline to the latest release (or install it if missing), using the right channel for this OS
---

# Update cc-probeline: /cc-probeline-update

Brings **cc-probeline** to the latest release. This is the command the status line
points to when it shows `↑ update: vX → vY — run /cc-probeline-update`. If the
binary is already installed it upgrades through whichever channel it came from
(Homebrew / Scoop / install script); **if it is not installed yet, this command
just installs it** — the user does not need to run a separate install command.
Every channel **shows the exact command and runs it only after the user approves
the native Bash permission prompt.** Never installs or upgrades silently.

## Usage

```
/cc-probeline-update
```

No arguments.

---

## Step 0 — Detect current state

```bash
command -v cc-probeline && cc-probeline --version; cc-probeline --check 2>/dev/null
```

- **Found** → note the current version; this run will **upgrade**.
- **Not found in PATH** → this run is a fresh **install** (do NOT send the user
  elsewhere — proceed through the same steps with the install variant).

Remember which case you are in: it only changes the single command in Step 2.

## Step 1 — Detect platform and the channel

```bash
uname -s; command -v brew && brew list --cask cc-probeline 2>/dev/null; command -v scoop && scoop list 2>/dev/null | grep -i cc-probeline
```

- Already managed by **Homebrew** (or `brew` exists and user is on macOS/Linux) → 2a.
- Already managed by **Scoop** (or Windows) → 2b.
- Neither package manager → **install script** (2c).

When installing fresh, prefer Homebrew on macOS/Linux if `brew` exists, Scoop on
Windows, otherwise the install script.

## Step 2 — Run it (pick exactly one channel)

**Show the user the exact command first, then run it via the Bash tool** — the Bash
permission prompt is the user's explicit "yes". Keep the command alone in its own
call so the prompt stays readable.

### 2a — Homebrew (macOS / Linux)

- Upgrade (binary already installed):

  ```bash
  brew upgrade --cask labzink/homebrew-tap/cc-probeline
  ```

- Fresh install (binary missing):

  ```bash
  brew install --cask labzink/homebrew-tap/cc-probeline
  ```

Either way the status line may not be wired yet → continue to Step 3.

### 2b — Scoop (Windows)

- Upgrade:

  ```bash
  scoop update cc-probeline
  ```

- Fresh install:

  ```bash
  scoop bucket add labzink https://github.com/labzink/scoop-bucket
  scoop install cc-probeline
  ```

A fresh Scoop install is not wired yet → continue to Step 3.

### 2c — Install script (macOS / Linux, no package manager)

The script fetches the latest release, verifies its SHA-256, installs over any
existing binary, **and wires the status line** — so it covers both upgrade and
fresh install in one step:

```bash
curl -fsSL https://raw.githubusercontent.com/labzink/cc-probeline/main/scripts/install.sh | sh
```

To pin a specific version instead of latest, prefix `CC_PROBELINE_VERSION=0.1.1`.
The script wires the status line itself, so **skip Step 3** and go to Step 4.

## Step 3 — Wire the status line (Homebrew / Scoop only)

Skip this if Step 0 already reported `installation OK` (an upgrade keeps the
existing wiring). Otherwise — a fresh Homebrew/Scoop install, or `--check` says the
status line is not wired — wire it:

```bash
cc-probeline install --merge-settings --binary-path "$(command -v cc-probeline)"
```

If a **different** status line is already configured, the binary asks before
overwriting and records a backup (restored on uninstall). Relay that and let the
user decide — do not force it.

## Step 4 — Verify and report

```bash
cc-probeline --version; cc-probeline --check
```

Expect `installation OK` and exit code 0. Then tell the user:

- the version now installed (compare with the one noted in Step 0 — say whether it
  was an upgrade, a fresh install, or already up to date),
- that the new status line / version takes effect on the next status-line refresh
  or in new Claude Code sessions,
- that `/cc-probeline-config` customises rows, colour, probes, and display mode.

If anything fails, report the exact error and stop — do not retry blindly.
