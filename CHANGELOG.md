# Changelog

All notable changes to `cc-probeline` are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Model-scoped weekly limit in the line** — plans that carry a per-model weekly cap (the "Fable" limit and its kin) now show it inside the 7-day block, in brackets: `7d: █████████▒ 95% (fable: ██████░░░░) ↻ 5d.0h`. Claude Code does not send this limit in the status-line feed at all — it exists only in its own usage cache — so cc-probeline reads it there. Accounts without such a limit see exactly the line they saw before.
- **Background limit refresh** (`[general].usage_refresh`, opt-out) — that cache is written by one thing only, Claude Code's `/usage` screen, so left alone it freezes for hours. cc-probeline now runs the screen headlessly (`claude -p "/usage" --no-session-persistence`) at most once every five minutes, and only for accounts that have a model-scoped limit or paid extra usage. It spends no model tokens and leaves no session behind. Turn it off with `cc-probeline usage-refresh off`.

### Changed

- **The extra-usage badge now shows Anthropic's own figure** — month-to-date spend against your ceiling (`+$20.40 / $120.00 extra usage`), with a dim age marker (`⏱ 2m`) because it comes from a cache. It replaces the previous session-local estimate, which was the one number in that row that was ours rather than Anthropic's. A snapshot older than an hour hides the badge instead of showing a stale figure.
- **`/cc-probeline-config`** swaps the colour toggle for the background-limit-refresh toggle; colour remains available as `cc-probeline no-color on|off`.

## [0.1.3] — 2026-06-17

### Changed

- **One-step install on every channel** — Homebrew and Scoop now wire the Claude Code status line on install, the same way the curl script already does. Install through any channel, restart Claude Code, and you're done — no separate wiring command. An existing custom status line is never overwritten (switch explicitly with `cc-probeline install --merge-settings --force`).
- **Uninstall restores your status line everywhere** — `brew uninstall` now restores the status line you had before removing the binary, and the curl script gained `--uninstall` (`install.sh … | sh -s -- --uninstall`) to do the same. On Scoop, run `cc-probeline uninstall` before `scoop uninstall` (Scoop has no uninstall hook).

## [0.1.2] — 2026-06-17

### Fixed

- **macOS `brew install`** — the Homebrew cask now strips the `com.apple.quarantine` attribute on install, so Gatekeeper no longer blocks the unsigned binary ("cannot verify the developer" / `zsh: killed`). `brew install` works out of the box on macOS again.

## [0.1.1] — 2026-06-17

### Added

- **Self-healing prices + update notice** — cc-probeline refreshes its price table over the network (one optional check a day, opt-out, never during render) so cost estimates track Anthropic's rates without a reinstall; offline or opted out, it uses the table baked in at build time. When a newer release is out, the status line shows an `↑ update: vX → vY — run /cc-probeline-update` hint. Disable the check with `price_check = false` or via the `/cc-probeline-config` wizard.
- **Update from the plugin** — a `/cc-probeline-update` command upgrades the binary through the channel it was installed with (Homebrew / Scoop / curl), or installs it if missing.
- **Install from the plugin** — the marketplace plugin ships a `/cc-probeline-install` command that detects your OS, installs the binary through the right channel (Homebrew / Scoop / curl) and wires the status line, asking before it runs anything. A session-start check offers it automatically when the binary is missing.
- **Build provenance** — release archives are signed with keyless build provenance attestation, verifiable with `gh attestation verify <file> --repo labzink/cc-probeline`.

### Changed

- **Config edits preserve comments** — toggling `tutorial_hints` (and the `/cc-probeline-config` wizard) now edits the value in place, keeping comments, formatting, and key order in your `config.toml` intact.

### Fixed

- **curl install path** — the documented `curl … | sh` one-liner now points at `scripts/install.sh` (the published location).

## [0.1.0] — 2026-06-16

First public release.

### Added

- **Status line core** — reads the active session JSONL from `~/.claude/projects/<slug>/*.jsonl` and renders a single status line: current model, token usage (input / output / cache_read / cache_create), and approximate session cost.
- **Per-turn table** — compact breakdown of cost and tokens per turn, with a configurable number of rows.
- **Context window indicator** — shows how much of the model context window the session is using.
- **Quota warnings** — block-limit and weekly rate-limit indicators derived from the official Claude Code source data (delta only, no bundled pricing tables).
- **Active subagent indicator** — surfaces subagents running in the session.
- **Probes** — model, cost, project, email, time, context, quota, and git, each individually toggleable.
- **Semantic colour** — 16-colour ANSI palette readable on both light and dark terminals, with `NO_COLOR` support and a monochrome mode.
- **Configuration** — TOML config file plus a `cc-probeline config` CLI and the `/cc-probeline-config` interactive wizard (also shipped as a plugin command).
- **Distribution** — Homebrew tap, Scoop bucket, and `install.sh` / `install.ps1` installers; GitHub Releases with SHA256 checksums for five targets (darwin arm64/amd64, linux arm64/amd64, windows amd64).
- **Plugin marketplace** — `.claude-plugin/marketplace.json` + `plugin.json` so the plugin is installable via `/plugin marketplace add labzink/cc-probeline` and `/plugin install cc-probeline@cc-probeline`.

### Notes

- The main status line is wired into Claude Code by `install.sh` / Homebrew / Scoop (or manually in `settings.json`). Claude Code plugins cannot set the main `statusLine` automatically, so the marketplace plugin provides discovery and the `/cc-probeline-config` wizard rather than auto-installing the status line.
