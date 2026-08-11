# Privacy Policy

**Effective date:** 2026-06-18

cc-probeline is a status-line plugin for Claude Code. This policy explains, in plain terms, what it does and does not do with your data. The short version: **cc-probeline collects nothing about you, sends no telemetry, and works entirely from data already on your machine.**

The source code is MIT-licensed and public, so every statement here can be verified by reading it.

## What we do NOT collect

cc-probeline has no telemetry, no analytics, and no tracking of any kind. It does not collect, store, or transmit:

- personal information;
- your prompts, code, or session contents;
- usage statistics or behavioral data;
- crash reports.

There is no account, no sign-in, and no user identifier.

## Data handled locally (never transmitted)

To render the status line, cc-probeline reads — locally, on your machine — only:

- your active session log (`~/.claude/projects/…/*.jsonl`);
- the status-line payload Claude Code pipes to it on each render;
- a single boolean field (`oauthAccount.hasExtraUsageEnabled`) from `~/.claude.json`, used only to show the extra-usage indicator. It does not read or transmit OAuth tokens or any other field from that file.

This data is processed in memory to draw the line and is never sent anywhere. cc-probeline does not read your credentials, keychain, OAuth tokens, `.env`, `.ssh`, or `.aws` files.

## Network activity

Rendering the status line is fully offline. The only network request cc-probeline itself makes is:

- **An optional, opt-out price/version check**, at most once every 24 hours, which downloads a single public JSON file hosted on GitHub. It is a plain file download — it sends nothing about you or your session: no identifiers, no usage, no query over your data. It exists only so displayed prices and the update hint stay current.

You can disable it with `cc-probeline price-check off`, after which cc-probeline makes no network request of its own. It never contacts the Anthropic API itself.

There is one further background action, which is not a request by cc-probeline but a request it *causes*:

- **An optional, opt-out limit refresh**, at most once every five minutes, and only for accounts that have a model-scoped weekly limit or paid extra usage. cc-probeline runs `claude -p "/usage" --no-session-persistence` in the background — the same usage screen you can open yourself, headless. Claude Code then talks to Anthropic with your own credentials, exactly as it does when you type `/usage`, and writes the result into its own cache in `~/.claude.json`, which cc-probeline reads. cc-probeline sends nothing, receives nothing, and never sees your token; it also costs no model tokens, because the usage screen calls no model. It exists because that cache is written by nothing else — without it those limits freeze for hours and the status line would show stale numbers.

Disable it with `cc-probeline usage-refresh off` (or in the `/cc-probeline-config` wizard), after which cc-probeline launches nothing and reads only whatever cache already exists.

## Third-party services

cc-probeline integrates with no analytics, crash-reporting, or tracking services. The only external endpoint it contacts itself is the public price/version JSON on GitHub (see above), and only when the opt-out check is enabled. That download is subject to GitHub's own privacy practices; cc-probeline includes no data of yours in the request beyond what any file download requires. The limit refresh contacts nothing directly — it starts Claude Code, which talks to Anthropic under its own privacy terms, as it already does throughout your session.

## Open source and auditable

cc-probeline is MIT-licensed and fully open source — you can audit exactly what it reads and sends by reading the code at <https://github.com/labzink/cc-probeline>.

Release binaries are built from that source in public CI and carry signed SLSA build provenance plus SHA-256 checksums. This lets anyone cryptographically verify that a downloaded release was built from this repository and was not tampered with in transit:

```
gh attestation verify <archive> --repo labzink/cc-probeline
```

In other words, none of the privacy claims above rest on trusting us — both the source code and the build chain are independently inspectable.

## Changes to this policy

If this policy changes, the updated version will be published in this file in the repository, with a new effective date. Material changes will also be noted in the project changelog.

## Contact

Questions about this policy: open an issue at <https://github.com/labzink/cc-probeline/issues> or email labzin.k@gmail.com.
