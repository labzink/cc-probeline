---
description: Update cc-probeline to the latest release (or install it if missing), using the right channel for this OS
---

# Update cc-probeline: /cc-probeline-update

Updates **cc-probeline** to the latest release through whichever channel fits this
machine (Homebrew / Scoop / install script). If it isn't installed yet, this same
command installs it. Always show the exact command and run it only after the user
approves the Bash permission prompt — never silently.

**Keep it simple: detect the channel, run one update command, report the result.**
Do **not** investigate config files, caches, state, or *why* an update hint
appeared — just update. If the tool is already on the latest version, say so and
stop. Do not re-run the same command hoping for a different result.

## Step 1 — Pick the channel

```bash
command -v brew scoop
```

- `brew` present → Homebrew (2a)
- `scoop` present (Windows) → Scoop (2b)
- neither → install script (2c) — the default on Linux/macOS

## Step 2 — Run one command (pick the matching channel)

Show the command, then run it via Bash so the permission prompt is the user's "yes".

### 2a — Homebrew

```bash
brew upgrade --cask labzink/homebrew-tap/cc-probeline || brew install --cask labzink/homebrew-tap/cc-probeline
```

Then, **only if it was a brand-new install** (status line not wired yet):

```bash
cc-probeline install --merge-settings
```

### 2b — Scoop (Windows)

```bash
scoop update cc-probeline || { scoop bucket add labzink https://github.com/labzink/scoop-bucket; scoop install cc-probeline; }
```

Then, **only on a brand-new install**:

```bash
cc-probeline install --merge-settings
```

### 2c — Install script (default — Linux/macOS, no package manager)

One command handles upgrade, fresh install, already-latest, and status-line wiring:

```bash
curl -fsSL https://raw.githubusercontent.com/labzink/cc-probeline/main/scripts/install.sh | sh
```

If it prints `already the latest version`, you are done — report that and stop.

## Step 3 — Report (one line)

State the installed version and whether it was an **upgrade**, a **fresh install**,
or **already up to date**. The new version applies on the next status-line refresh
or in new Claude Code sessions. Mention that `/cc-probeline-config` customises rows,
colour, probes and display mode. If a command fails, show the exact error and
stop — do not retry blindly.
