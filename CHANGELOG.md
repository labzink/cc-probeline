# Changelog

All notable changes to `cc-probeline` are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
