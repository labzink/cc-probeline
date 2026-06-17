---
description: Update the installed cc-probeline binary to the latest release, using the same channel it was installed through
---

# Update cc-probeline: /cc-probeline-update

Updates the **cc-probeline** binary to the latest release. This is the command the
status line points to when it shows `↑ update: vX → vY — run /cc-probeline-update`.
It upgrades through whichever channel the binary was installed with (Homebrew /
Scoop / install script), **shows the exact command, and runs it only after the
user approves the native Bash permission prompt.** Never upgrades silently.

## Usage

```
/cc-probeline-update
```

No arguments.

---

## Step 0 — Confirm it is installed

```bash
command -v cc-probeline && cc-probeline --version
```

- **Not found in PATH** → there is nothing to update. Tell the user it is not
  installed yet and to run **`/cc-probeline-install`** instead, then stop.
- **Found** → note the current version and continue.

## Step 1 — Detect the channel it was installed through

```bash
uname -s; command -v brew && brew list --cask cc-probeline 2>/dev/null; command -v scoop && scoop list 2>/dev/null | grep -i cc-probeline
```

- Listed by **Homebrew** (`brew list --cask` shows it) → Step 2a.
- Listed by **Scoop** → Step 2b.
- Neither (installed via the script, or unknown) → Step 2c.

## Step 2 — Run the upgrade

Pick exactly one channel. **Show the user the exact command first, then run it via
the Bash tool** — the Bash permission prompt is the user's explicit "yes". Keep the
upgrade command alone in its own call so the prompt is readable.

### 2a — Homebrew (macOS / Linux)

```bash
brew upgrade --cask labzink/homebrew-tap/cc-probeline
```

### 2b — Scoop (Windows)

```bash
scoop update cc-probeline
```

### 2c — Install script (no package manager)

The install script always fetches the latest release, verifies its SHA-256, and
re-installs over the existing binary:

```bash
curl -fsSL https://raw.githubusercontent.com/labzink/cc-probeline/main/scripts/install.sh | sh
```

If the package manager reports the binary is **already up to date**, relay that to
the user — no update was needed (the status line may simply be a refresh behind).

## Step 3 — Verify and report

```bash
cc-probeline --version; cc-probeline --check
```

Tell the user the new version (compare with the one noted in Step 0) and confirm
`--check` reports `installation OK`. The status line keeps its existing wiring and
config — an update only swaps the binary. The new version takes effect on the next
status-line refresh or in new Claude Code sessions.

If the upgrade fails, report the exact error and stop — do not retry blindly.
