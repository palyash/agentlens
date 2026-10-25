# agentlens

A local, read-only dashboard for your [Claude Code](https://docs.claude.com/en/docs/claude-code) sessions. It shows where your tokens go, what your tools return into context, which commands fail or retry, and ranks a few concrete fixes with ready-to-paste CLAUDE.md snippets.

Single static binary. Standard library only. Nothing leaves your machine.

> Not affiliated with or endorsed by Anthropic.

## Quick start

```bash
go install github.com/palyash/agentlens/cmd/agentlens@latest

agentlens -demo      # try it on bundled sample data
agentlens            # read your own transcripts (~/.claude/projects)
```

Then open http://127.0.0.1:8080.

| Flag | Meaning |
|---|---|
| `-root DIR` | transcript folder (default `$CLAUDE_CONFIG_DIR/projects` or `~/.claude/projects`) |
| `-port N` | port, bound to localhost only (default 8080) |
| `-demo` | use bundled synthetic data |
| `doctor` | print what was parsed, which models lack a price, and how many tool calls errored |

## What you get

- **Fix this first**: ranked recommendations with evidence and a copyable fix (noisy command output, retried failures, repeated environment setup, cache rebuilds after idle gaps, cheaper model for light requests).
- **Overview**: spend by day, model, project and token type; cache hit rate.
- **Tools and insights**: per-tool usage and failures, Bash by command family, largest outputs, tool usage per day.
- **Sessions**: request-by-request view of any session.

## Privacy

It only reads your transcript folder. It writes nothing, makes no network calls, and listens on `127.0.0.1`. The server has GET routes only.

## How the numbers are computed

These are estimates, not billed amounts.

- **Cost** is token counts times the list prices in `internal/pricing`. Enterprise or subscription plans may bill differently. Check the table against current pricing.
- **Tool tokens** are the result size divided by 4.
- **Carry cost** is the estimated cost of keeping a tool result in context: its tokens times the number of later requests times the cache-read price. It is why a large `go test` log costs more than the one call suggests.
- **Savings per week** in the recommendations scale the above by the time span of your data.

## Limits

- The transcript format is undocumented and can change between Claude Code releases. `agentlens doctor` shows what was understood. Regression tests run against synthetic fixtures only.
- Resumed sessions may be counted twice.
- Only Claude Code is supported today.

## Development

```bash
go test ./...
```

`internal/analyze/testdata/golden.json` was produced by the original Python prototype on the same fixtures, so the Go port is pinned to the reference implementation. Never commit real transcripts: they contain your code and prompts.

Layout: `cmd/agentlens` (CLI) · `internal/transcript` (parsing) · `internal/pricing` · `internal/analyze` (report and detectors) · `internal/server` (HTTP) · `internal/web` (embedded UI) · `internal/demo` (sample data).

## Roadmap

- A "where your time goes" view (tool runtime, waiting, failure loops)
- A second adapter (Codex CLI) behind the same internal model

## License

MIT
