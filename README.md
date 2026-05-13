# cc-probeline

> **Probe** your Claude Code session — every token, every dependency, in plain sight.

`cc-probeline` is a status line plugin for Claude Code that surfaces what the agent is doing right now: model, token usage (input / output / cache_read / cache_create), approximate session cost, active subagents, warnings.

**Status:** work in progress. First public release coming after Phase 2 core development. Public repo is private during scaffold-up; flips to public on first usable release.

## Why "probe"?

Probes — as in eBPF, DTrace, sysdig — are tools that let you see deep inside a running process. `cc-probeline` does the same for your Claude Code session: zero hidden network calls, source visible from day one, reproducible builds with GitHub Attestations, MIT license. The whole point is auditability.

## Planned features (subject to change)

- Live token + cost display per session
- Active subagent indicator
- Cache hit / cache create breakdown
- Rate-limit warnings (block-limit and weekly)
- No silent auto-updates, no postinstall hooks, no OAuth-token reads

## Roadmap

See `CHANGELOG.md` for released versions. The detailed roadmap and internal R&D notes live in the development repo and are not published here.

## License

MIT. See `LICENSE`.

<!-- pipeline smoke test marker — safe to remove -->
