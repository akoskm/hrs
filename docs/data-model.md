# hrs — Data Model

## Core Principle

**Time entries are the center of everything.** Each entry carries its own context.
There is no session entity. Entries from agents and entries from humans are the
same thing — just with different `operator` values.

## Entity Relationship

```
┌──────────┐       ┌──────────────┐       ┌──────────┐
│  Client   │──1:N──│   Project    │──1:N──│   Task   │
└──────────┘       └──────────────┘       └──────────┘
                          │                      │
                          │ (nullable)           │ (nullable)
                          │                      │
                          │                ┌─────┴──────┐
                          └───────────────►│ TimeEntry  │◄── Labels (M:N)
                                           └────────────┘
                                                 │
                                           ┌─────┴──────┐
                                           │ ImportLog  │
                                           └────────────┘

  ┌──────────────┐
  │ ProjectPath  │  (cwd → project auto-detection)
  └──────────────┘
```

## How Parallel Work is Tracked

Multiple time entries can overlap in wall-clock time. There is no constraint
preventing this. At any point you might have:

- One human entry (you in a meeting)
- Two Claude Code entries (two worktrees running in parallel)
- One Codex entry (async cloud task)

All four overlap. All four are just rows in time_entries with different
`operator` values. Queries split by `operator` for human vs agent breakdowns.

## Tables

### clients
| Column       | Type    | Notes                |
|-------------|---------|----------------------|
| id          | TEXT PK | ULID                 |
| name        | TEXT    | NOT NULL, UNIQUE     |
| contact     | TEXT    | optional             |
| archived_at | TEXT    | soft archive         |
| created_at  | TEXT    | ISO 8601             |
| updated_at  | TEXT    | ISO 8601             |

### projects
| Column           | Type    | Notes                                  |
|-----------------|---------|-----------------------------------------|
| id              | TEXT PK | ULID                                    |
| client_id       | TEXT FK | nullable                                |
| name            | TEXT    | NOT NULL                                |
| code            | TEXT    | UNIQUE, short CLI identifier            |
| hourly_rate     | INTEGER | smallest currency unit (cents/fillér)   |
| currency        | TEXT    | ISO 4217: HUF, CHF, EUR                |
| billable_default| INTEGER | 0 or 1                                  |
| color           | TEXT    | hex for TUI charts                      |
| archived_at     | TEXT    | soft archive                            |
| created_at      | TEXT    | ISO 8601                                |
| updated_at      | TEXT    | ISO 8601                                |

### tasks
| Column      | Type    | Notes                             |
|------------|---------|-----------------------------------|
| id         | TEXT PK | ULID                              |
| project_id | TEXT FK | NOT NULL                          |
| name       | TEXT    | NOT NULL                          |
| code       | TEXT    | unique per project                |
| done       | INTEGER | 0 or 1                            |
| archived_at| TEXT    | soft archive                      |
| created_at | TEXT    | ISO 8601                          |
| updated_at | TEXT    | ISO 8601                          |

### time_entries
| Column        | Type    | Notes                                          |
|--------------|---------|------------------------------------------------|
| id           | TEXT PK | ULID                                           |
| project_id   | TEXT FK | **nullable** — unassigned drafts from sync      |
| task_id      | TEXT FK | nullable                                       |
| description  | TEXT    | what was done                                  |
| started_at   | TEXT    | ISO 8601                                       |
| ended_at     | TEXT    | ISO 8601                                       |
| duration_secs| INTEGER | cached, computed from start/end                |
| billable     | INTEGER | 0 or 1                                         |
| status       | TEXT    | 'draft' or 'confirmed'                         |
| operator     | TEXT    | 'human', 'claude-code', 'codex', etc.          |
| source_ref   | TEXT    | external session ID for import dedup            |
| worktree     | TEXT    | worktree name (from agent logs)                |
| git_branch   | TEXT    | branch name (from agent logs)                  |
| cwd          | TEXT    | working directory (for project auto-detection) |
| metadata     | TEXT    | JSON blob: tokens, model, tool counts          |
| deleted_at   | TEXT    | soft delete                                    |
| created_at   | TEXT    | ISO 8601                                       |
| updated_at   | TEXT    | ISO 8601                                       |

### labels
| Column     | Type    | Notes              |
|-----------|---------|---------------------|
| id        | TEXT PK | ULID                |
| name      | TEXT    | NOT NULL, UNIQUE    |
| color     | TEXT    | hex                 |
| created_at| TEXT    | ISO 8601            |

### time_entry_labels
| Column         | Type    | Notes  |
|---------------|---------|--------|
| time_entry_id | TEXT FK | PK leg |
| label_id      | TEXT FK | PK leg |

### import_log
| Column          | Type    | Notes                             |
|----------------|---------|-----------------------------------|
| id             | TEXT PK | ULID                              |
| source         | TEXT    | 'claude-code', 'codex', etc.      |
| source_path    | TEXT    | file path imported                |
| session_id     | TEXT    | external session ID               |
| entries_created| INTEGER | count                             |
| imported_at    | TEXT    | ISO 8601                          |

### project_paths
| Column     | Type    | Notes                    |
|-----------|---------|--------------------------|
| id        | TEXT PK | ULID                     |
| project_id| TEXT FK | NOT NULL                 |
| path      | TEXT    | absolute path, UNIQUE    |
| created_at| TEXT    | ISO 8601                 |

## Key Statistics Queries

```sql
-- Hours per project this week
SELECT p.name, p.code, p.currency,
       SUM(te.duration_secs) / 3600.0 AS hours,
       SUM(te.duration_secs) / 3600.0 * p.hourly_rate / 100.0 AS earnings
FROM time_entries te
JOIN projects p ON p.id = te.project_id
WHERE te.started_at >= date('now', 'weekday 0', '-7 days')
  AND te.deleted_at IS NULL AND te.status = 'confirmed'
GROUP BY p.id;

-- Human vs agent hours
SELECT te.operator,
       SUM(te.duration_secs) / 3600.0 AS hours,
       COUNT(*) AS entries
FROM time_entries te
WHERE te.started_at >= date('now', 'weekday 0', '-7 days')
  AND te.deleted_at IS NULL
GROUP BY te.operator;

-- Daily breakdown (for charts)
SELECT date(te.started_at) AS day,
       SUM(te.duration_secs) / 3600.0 AS total_hours,
       SUM(CASE WHEN te.billable = 1 THEN te.duration_secs ELSE 0 END) / 3600.0 AS billable_hours,
       SUM(CASE WHEN te.operator = 'human' THEN te.duration_secs ELSE 0 END) / 3600.0 AS human_hours,
       SUM(CASE WHEN te.operator != 'human' THEN te.duration_secs ELSE 0 END) / 3600.0 AS agent_hours
FROM time_entries te
WHERE te.started_at >= date('now', '-30 days') AND te.deleted_at IS NULL
GROUP BY day ORDER BY day;

-- Unassigned entries (for review TUI)
SELECT te.* FROM time_entries te
WHERE te.project_id IS NULL AND te.deleted_at IS NULL
ORDER BY te.started_at;

-- Billable report per client
SELECT c.name AS client, p.name AS project, p.currency,
       SUM(CASE WHEN te.billable = 1 THEN te.duration_secs ELSE 0 END) / 3600.0 AS billable_h,
       SUM(CASE WHEN te.billable = 1 THEN te.duration_secs ELSE 0 END) / 3600.0 * p.hourly_rate / 100.0 AS earnings
FROM time_entries te
JOIN projects p ON p.id = te.project_id
LEFT JOIN clients c ON c.id = p.client_id
WHERE te.deleted_at IS NULL AND te.status = 'confirmed'
GROUP BY c.id, p.id;
```

## Design Decisions

### Why no sessions table?
Sessions added complexity without value. Time entries carry all the context
they need: operator, worktree, git_branch, cwd, source_ref, metadata.
Parallel work is just overlapping entries with different operator values.

### Why nullable project_id?
Agent-imported entries start as unassigned drafts. The TUI's job is to move
them from draft → confirmed by assigning a project. This matches the real
workflow: sync first, classify later.

### Why status (draft/confirmed)?
Reports need to distinguish reviewed data from raw imports. You don't want
unreviewed agent entries inflating your billable hours report.

### Why rates as integers?
15000 = 150.00 CHF. No floating point rounding in billing math.

### Why soft deletes?
Billing disputes need audit trails. Undo is trivial.

### Why ULIDs?
Sortable by creation time, no central sequence, safe for future sync scenarios.
