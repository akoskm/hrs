# hrs

A TUI time tracker that imports coding agent sessions (Claude Code, Codex, OpenCode) and lets you turn them into billable time entries.

![Go](https://img.shields.io/badge/Go-1.24-blue) ![SQLite](https://img.shields.io/badge/SQLite-embedded-green)

## What it does

1. Syncs agent activity from JSONL logs into a timeline
2. Shows activity markers so you see when and where agents were working
3. You create time entries informed by the markers, assign projects, mark billable/non-billable
4. Generate reports for invoicing

Works without agents too — add manual entries for meetings, reviews, etc.

## Install

```bash
go install github.com/akoskm/hrs@latest
```

Or build from source:

```bash
git clone https://github.com/akoskm/hrs.git
cd hrs
go build -o hrs .
```

## Quick start

```bash
# Set up a client and project
hrs client add "Acme Corp"
hrs project add "Website" --code website --client "Acme Corp" --rate 10000 --currency USD

# Map a local directory to the project (for auto-detection during sync)
hrs path add /path/to/website-repo website

# Sync agent logs
hrs sync              # all sources
hrs sync claude       # Claude Code only
hrs sync codex        # Codex only
hrs sync opencode     # OpenCode only

# Open the TUI
hrs

# Or add entries from the command line
hrs add --project website --from 9:00 --to 11:00 "Sprint planning"
```

## Database

Stored at `~/.config/hrs/hrs.db`. Override with `--db` flag or `HRS_DB` env var.

## Key bindings (TUI)

| Key | Action |
|-----|--------|
| `j/k` | Navigate entries |
| `n/t` | Next/previous day |
| `a` | Add entry in gap |
| `e` | Edit entry |
| `d` | Delete entry |
| `s` | Sync now |
| `Tab` | Switch tabs |
| `q` | Quit |

## Running tests

```bash
go test ./...
```

## License

MIT
