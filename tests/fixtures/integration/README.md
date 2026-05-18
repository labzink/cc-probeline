# Integration test fixtures

Anonymized snapshots of real Claude Code JSONL sessions used by
`tests/integration/parser_integration_test.go`.

## Files

| Fixture | Turns | Description |
|---------|-------|-------------|
| `real-session-short.jsonl` | 21 | Short opus-only session, no subagents |
| `real-session-medium.jsonl` | 25 | Longer opus-only session, higher cache traffic |
| `real-session-subagents.jsonl` | 30 | Orchestrator session with 5 subagents |
| `real-session-subagents/subagents/` | — | 5 subagent JSONL + meta.json pairs |

## How to regenerate

```
python3 scripts/anonymize-jsonl.py <source.jsonl> tests/fixtures/integration/<dest.jsonl>
python3 scripts/anonymize-jsonl.py <source.meta.json> tests/fixtures/integration/.../<dest.meta.json>
```

`--meta` is auto-detected for `.meta.json` inputs.

After regeneration, run `go test -tags=integration ./tests/integration/... -v`
to confirm golden values are unchanged.

## Golden values

Expected token counts, turn counts, and timestamps are embedded as constants
in `tests/integration/parser_integration_test.go`.
