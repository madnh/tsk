---
name: task
description: Use when starting work, finding next task, updating progress, or checking project status. Manages implementation tasks stored as markdown. Also use when the user says "task", "next task", "what to do", "progress", "board".
argument-hint: "<command> [options]"
allowed-tools: Bash(node *), Read
---

# Task Management

CLI tool for managing implementation tasks. Tasks are stored as markdown files in `tasks/items/`.

## CLI Base Command

```bash
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs <command> [options] -o json
```

Always use `-o json` so you can parse the output programmatically.

## Workflow: Starting a Session

When beginning work, follow this sequence:

**1. Find next available task:**
```bash
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs next -o json
```
Returns the highest-priority pending task that is not blocked. Response includes `spec` field pointing to the feature spec file.

**2. Claim the task:**
```bash
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs start TASK-001 -o json
```
Changes status from `pending` to `in_progress`. Will error if task is blocked by dependencies.

**3. Read the task details:**
```bash
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs show TASK-001 -o json
```
Returns full task including Description, Acceptance Criteria, and Log. The `body` field contains the markdown content.

**4. Read the linked spec file:**
Use the `spec` field from the task (e.g., `docs/features/database/spec.md`) and read it with the Read tool. This is the source of truth for what to implement.

## Workflow: During Implementation

**Log progress** as you complete significant steps:
```bash
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs log TASK-001 --stdin -o json << 'EOF'
- Implemented `Adapter` interface in `internal/database/adapter.go`
- Added `Query()`, `Exec()`, `Batch()` methods
- SQLite adapter with WAL mode in `internal/database/sqlite.go`
EOF
```

**Track files you create or modify:**
```bash
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs files TASK-001 --add "internal/database/adapter.go,internal/database/sqlite.go,internal/database/sqlite_test.go" -o json
```

Use `--stdin` with heredoc (`<< 'EOF'`) for log messages. This is safe for any content — special characters, backticks, dollar signs, multi-line text.

## Workflow: Completing Work

When all acceptance criteria are met:
```bash
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs done TASK-001 -o json
```
This sets status to `review`. The developer will then `approve` or `reject`.

## Workflow: After Rejection

If developer rejects, the task returns to `in_progress`. Read the task again to see the rejection feedback in the Log section, then address the issues:
```bash
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs show TASK-001 -o json
```

## Checking Status

```bash
# Dashboard: summary, active, review, blocked tasks
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs board -o json

# List available tasks (pending + not blocked)
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs list --available -o json

# Filter by phase or feature
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs list --phase 1 -o json
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs list --feature database -o json

# Progress per phase
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs progress -o json
```

## Dependencies

```bash
# View dependency tree of a task (what must be done before it)
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs deps TASK-005 -o json

# View reverse deps (what tasks depend on this one)
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs deps TASK-001 --reverse -o json
```

The `next` command automatically skips blocked tasks. The `start` command will error if dependencies are not met. Circular dependencies are detected and rejected when editing `--depends`.

## Creating Tasks

Developer creates tasks. If you need to create subtasks:
```bash
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs create --title "Implement connection pooling" --phase 1 --feature database --priority medium --depends TASK-002 --spec "docs/features/database/spec.md" --stdin -o json << 'EOF'
## Description

Add connection pooling to the SQLite adapter with configurable pool size.

## Acceptance Criteria

- [ ] Pool size configurable via config file
- [ ] Default pool: 1 writer + 4 readers
- [ ] Graceful shutdown drains pool
- [ ] Unit tests for pool behavior
EOF
```

Required fields: `--title`, `--phase`, `--feature`.
Body must contain `## Description` and `## Acceptance Criteria` sections.

## Status Flow

```
pending → in_progress → review → done
                ↑                  |
                └── (reject) ──────┘
```

- `start`: pending → in_progress
- `done`: in_progress → review
- `approve`: review → done (developer only)
- `reject`: review → in_progress (developer only)

Status can only change through these commands. Direct status editing is blocked.

## Task File Structure

Tasks are stored in `tasks/items/TASK-NNN.md` with frontmatter:
```yaml
id, title, status, phase, feature, priority, depends, spec, files, created, started, completed
```

Body sections: `## Description`, `## Acceptance Criteria`, `## Log` (append-only).

## Autonomous Loop (Ralph)

Ralph is an autonomous AI execution loop that cycles through **analyze → implement → review** to complete pre-defined tasks. See [RALPH.md](RALPH.md) for full documentation.

Quick start: `bash ${CLAUDE_SKILL_DIR}/scripts/ralph.sh`

## Rules

1. Always read the spec file before implementing
2. Log progress after each significant step (not every small change)
3. Track all created/modified files with the `files` command
4. Use `--stdin` with `<< 'EOF'` for any multi-line or special-character content
5. Only mark `done` when ALL acceptance criteria are met
6. If blocked, skip to the next available task
7. Do not modify task files directly — always use CLI commands