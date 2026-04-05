# hrs — Time Tracker for the Agentic Era

## What is hrs?

hrs is a TUI-first time tracking tool. It passively collects coding agent sessions
(Claude Code, Codex) from their log files, presents them as a visual timeline, and
gives you an interactive interface to assign them to projects/tasks and fill in
manual time blocks. The output is accurate billable hours reports.

## Core Workflow

1. You work all day — coding agents come and go, you do meetings and reviews
2. End of day (or whenever), you run `hrs`
3. The timeline shows **activity markers** from agent logs — ticks/dots showing when agents were active, not pre-created time entries. Agents don't create entries; they provide context.
4. You see where work happened and **manually create time entries** informed by the activity markers
5. You assign entries to projects, optionally tasks
6. You add manual entries for meetings, reviews, etc.
7. You mark things billable/non-billable
8. Reports and invoicing data flow from there

## Agent Activity vs Time Entries (Key Distinction)

Agent logs are **raw activity data**, not time entries. Like traditional time trackers
show app usage as background context, hrs shows agent message timestamps as visual
indicators on the timeline. The user decides what becomes a billable time entry.

- **Sync reads agent logs** → extracts individual message timestamps
- **Timeline shows activity markers** → background ticks/dots, not entries
- **User creates entries** → informed by seeing when agents were active and what they did
- **Sessions ≠ entries** — a session can span days; what matters is when messages happened

## The Tool is Fully Usable Without Agents

Agent sync is just one source of data. You can use hrs purely with manual entries:
`hrs add --project myproject --from 9:00 --to 11:00 "Sprint planning meeting"`

## Tech Stack

- **Language**: Go
- **CLI framework**: Cobra + Viper
- **TUI framework**: Bubble Tea (charmbracelet/bubbletea) + Lip Gloss + Bubbles
- **Charts**: termui (gizak/termui/v3) for bar charts, sparklines, pie charts
- **Database**: SQLite via modernc.org/sqlite (pure Go, no CGO)
- **IDs**: ULID via oklog/ulid
- **Testing**: Go standard testing + bubbletea/teatest for TUI tests

## Project Structure

```
hrs/
├── CLAUDE.md              # this file
├── go.mod
├── go.sum
├── main.go                # just calls cmd.Execute()
├── cmd/                   # Cobra commands
│   ├── root.go            # root command, launches TUI by default
│   ├── add.go             # manual time entry
│   ├── sync.go            # import agent logs
│   ├── review.go          # classify unassigned entries
│   ├── report.go          # generate reports
│   ├── project.go         # manage projects (add/list/archive)
│   ├── client.go          # manage clients
│   ├── task.go            # manage tasks
│   ├── label.go           # manage labels
│   └── path.go            # manage project path mappings
├── internal/
│   ├── db/
│   │   ├── db.go          # open/migrate/close
│   │   ├── migrations/
│   │   │   └── 001_initial_schema.sql
│   │   ├── clients.go     # client CRUD
│   │   ├── projects.go    # project CRUD
│   │   ├── tasks.go       # task CRUD
│   │   ├── entries.go     # time entry CRUD + queries
│   │   ├── labels.go      # label CRUD + junction
│   │   ├── imports.go     # import log operations
│   │   └── paths.go       # project path mappings
│   ├── model/
│   │   └── model.go       # domain types
│   ├── sync/
│   │   ├── claude.go      # Claude Code JSONL parser + importer
│   │   ├── codex.go       # Codex session parser + importer
│   │   └── detect.go      # cwd → project auto-detection
│   └── tui/
│       ├── app.go          # main TUI application
│       ├── timeline.go     # timeline view
│       ├── review.go       # entry assignment view
│       ├── report.go       # report/chart view
│       └── components/     # reusable TUI components
├── testdata/
│   └── claude-sessions/    # fixture JSONL files for testing
└── docs/
    └── data-model.md       # schema documentation
```

## Data Model

See docs/data-model.md for full documentation. Key points:

- **Clients → Projects → Tasks** hierarchy
- **Time entries** are the center — each carries its own context
- `project_id` is **nullable** — entries from agent sync start as unassigned drafts
- `status` field: `draft` (auto-imported) or `confirmed` (user reviewed)
- `operator` field: `human` for manual entries, agent name for imported
- Multiple entries can overlap in wall-clock time (you + parallel agents)
- No session entity — entries are self-contained
- `project_paths` table maps cwds to projects for auto-detection
- `import_log` table for idempotent re-syncs
- Rates stored as integers in smallest currency unit (cents/fillér)
- Soft deletes via `deleted_at` on time entries
- All IDs are ULIDs (sortable, no collisions)

## Database Location

`~/.config/hrs/hrs.db` (XDG compliant), overridable with `--db` flag or `HRS_DB` env var.

## Agent Log Locations

- Claude Code: `~/.claude/projects/<encoded-cwd>/*.jsonl`
- Claude Code index: `~/.claude/projects/<encoded-cwd>/sessions-index.json`
- Codex: `~/.codex/sessions/*.jsonl`

## Claude Code JSONL Format

Each line is a JSON object:
```json
{
  "parentUuid": "string",
  "sessionId": "string",
  "version": "string",
  "gitBranch": "string",
  "cwd": "string",
  "message": {
    "role": "user|assistant",
    "content": [
      { "type": "text|tool_use|tool_result", "text": "string", ... }
    ],
    "usage": {
      "input_tokens": 0,
      "output_tokens": 0,
      "cache_creation_input_tokens": 0,
      "cache_read_input_tokens": 0
    }
  },
  "uuid": "string",
  "timestamp": "ISO-8601",
  "toolUseResult": {}
}
```

The `sessions-index.json` has summaries, message counts, git branches, timestamps.

## CLI Commands

```bash
# Default: open TUI dashboard
hrs

# Manual entry
hrs add --project elaiia --from 9:00 --to 11:00 "Sprint planning"
hrs add --project elaiia --from 9:00 --to 11:00 --billable=false "Internal sync"

# Sync agent logs
hrs sync                    # sync all sources
hrs sync claude             # Claude Code only
hrs sync codex              # Codex only

# Review unassigned entries in TUI
hrs review

# Reports
hrs report                  # current week summary
hrs report --week           # this week
hrs report --month          # this month
hrs report --from 2026-03-01 --to 2026-03-31
hrs report --project elaiia
hrs report --client "Delta Labs AG"
hrs report --json           # JSON output for scripting

# Entity management
hrs project add "Elaiia" --code elaiia --client "Delta Labs AG" --rate 15000 --currency CHF
hrs project list
hrs client add "Delta Labs AG"
hrs task add "Auth Refactor" --project elaiia --code auth
hrs label add coding --color "#00ff00"

# Path mappings for auto-detection
hrs path add /Users/akos/code/elaiia elaiia
hrs path add /Users/akos/code/delta-one-nextjs d1-next
hrs path list
```

## Testing Strategy

All features must be developed test-first. The test suite is the primary feedback loop.

### Layer 1: JSONL Parser (unit tests)
- Parse fixture files in `testdata/claude-sessions/`
- Extract session boundaries, timestamps, metadata
- Handle malformed/incomplete files gracefully

### Layer 2: Database (integration tests)
- In-memory SQLite for all DB tests
- Test schema creation and migrations
- Test all CRUD operations
- Test constraint enforcement (import dedup, unique codes, etc.)
- Test all statistics queries with known data
- Test nullable project_id (draft entries)

### Layer 3: Sync Pipeline (integration tests)
- Fixture JSONL → parse → insert → query → assert
- Test idempotent re-import (run twice, same result)
- Test project auto-detection from cwd
- Test worktree detection from path patterns

### Layer 4: TUI (unit + golden tests)
- Test Update() model logic directly (pure functions)
- Use teatest for headless TUI integration tests
- Golden file snapshots for view rendering

### Layer 5: CLI (integration tests)
- Test Cobra commands end-to-end
- Verify JSON output format for scripting

## Build Order

1. `internal/model/model.go` — domain types
2. `internal/db/` — schema, migrations, CRUD, queries
3. `internal/sync/claude.go` — JSONL parser
4. `internal/sync/detect.go` — cwd → project resolution
5. `cmd/` — Cobra commands (non-TUI first: add, sync, report, project, etc.)
6. `internal/tui/` — Bubble Tea interface
7. `cmd/root.go` — wire TUI as default command

## Code Style

- Use `RunE` (not `Run`) on Cobra commands for proper error propagation
- All DB operations take a `context.Context`
- Return errors, don't log and continue
- Use table-driven tests
- Keep TUI model logic separate from view rendering
