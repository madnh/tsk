---
name: task
description: Manage implementation tasks. Use when starting work, finding next task, updating progress, or checking project status. Also use when the user says "task", "next task", "what to do", "progress", "board".
argument-hint: "<command> [options]"
allowed-tools: Bash(tsk *), Read
---

# Task Management

CLI tool for managing implementation tasks. Tasks are stored as markdown files in `tasks/items/`.

## CLI Base Command

```bash
tsk <command> [options] -o json
```

Always use `-o json` so you can parse the output programmatically.

## Workflow: Starting a Session

When beginning work, follow this sequence:

**1. Find next available task:**
```bash
tsk next -o json
```
Returns the highest-priority pending task that is not blocked. Response includes `spec` field pointing to the feature spec file.

**2. Claim the task:**
```bash
tsk start TASK-001 -o json
```
Changes status from `pending` to `in_progress`. Will error if task is blocked by dependencies.

**3. Read the task details:**
```bash
tsk show TASK-001 -o json
```
Returns full task including Description, Acceptance Criteria, and Log. The `body` field contains the markdown content.

**4. Read the linked spec file:**
Use the `spec` field from the task (e.g., `docs/features/database/spec.md`) and read it with the Read tool. This is the source of truth for what to implement.

## Workflow: During Implementation

**Log progress** as you complete significant steps:
```bash
tsk log TASK-001 --stdin -o json << 'EOF'
- Implemented `Adapter` interface in `internal/database/adapter.go`
- Added `Query()`, `Exec()`, `Batch()` methods
- SQLite adapter with WAL mode in `internal/database/sqlite.go`
EOF
```

**Track files you create or modify:**
```bash
tsk files TASK-001 --add "internal/database/adapter.go,internal/database/sqlite.go,internal/database/sqlite_test.go" -o json
```

Use `--stdin` with heredoc (`<< 'EOF'`) for log messages. This is safe for any content — special characters, backticks, dollar signs, multi-line text.

## Workflow: Completing Work

When all acceptance criteria are met:
```bash
tsk done TASK-001 -o json
```
This sets status to `review`. The developer will then `approve` or `reject`.

## Workflow: After Rejection

If developer rejects, the task returns to `in_progress`. Read the task again to see the rejection feedback in the Log section, then address the issues:
```bash
tsk show TASK-001 -o json
```

## Phase Management

```bash
# List all phases
tsk phase -o json

# Show specific phase detail
tsk phase 1 -o json

# Create a new phase (auto-assigns next number)
tsk phase create --name "Setup" --description "Project setup" -o json
tsk phase create --name "Core" --status ready -o json

# Delete a phase (fails if tasks reference it)
tsk phase delete 3 -o json

# Sync phases from tsk.yml config
tsk phase sync -o json

# Update phase metadata
tsk phase 1 --status ready -o json
tsk phase 1 --name "New Name" --description "Updated" -o json

# Add log entry to phase
tsk phase log 1 --stdin -o json << 'EOF'
Completed initial setup, moving to implementation.
EOF
```

Phase must exist before creating tasks that reference it. Use `tsk phase create` or `tsk phase sync` first.

## Checking Status

```bash
# Dashboard: summary, active, review, blocked tasks
tsk board -o json

# List available tasks (pending + not blocked)
tsk list --available -o json

# Filter by phase or feature
tsk list --phase 1 -o json
tsk list --feature database -o json

# Progress per phase
tsk progress -o json
```

## Dependencies

```bash
# View dependency tree of a task (what must be done before it)
tsk deps TASK-005 -o json

# View reverse deps (what tasks depend on this one)
tsk deps TASK-001 --reverse -o json
```

The `next` command automatically skips blocked tasks. The `start` command will error if dependencies are not met. Circular dependencies are detected and rejected when editing `--depends`.

## Creating Tasks

Developer creates tasks. If you need to create subtasks:
```bash
tsk create --title "Implement connection pooling" --phase 1 --feature database --priority medium --depends TASK-002 --spec "docs/features/database/spec.md" --stdin -o json << 'EOF'
## Description

Add connection pooling to the SQLite adapter with configurable pool size.

## Acceptance Criteria

- [ ] Pool size configurable via `contentless.yaml`
- [ ] Default pool: 1 writer + 4 readers
- [ ] Graceful shutdown drains pool
- [ ] Unit tests for pool behavior
EOF
```

Required fields: `--title`, `--phase`, `--feature`.
The phase must already exist — create it first with `tsk phase create --name <name>` or `tsk phase sync`.
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

Ralph is an autonomous AI execution loop that cycles through **analyze → implement → review** to complete pre-defined tasks. It does NOT create tasks or decide what to build — the user defines tasks, Ralph executes them. Each step spawns a fresh `claude -p` session with a generated prompt — state persists only through files, not memory.

### Architecture

```
tsk ralph (Go orchestrator)
  │
  ├── loop init          → creates state.json
  │
  └── while running:
        ├── loop status  → display current state
        ├── loop prompt  → generate step-specific prompt
        ├── claude -p    → execute prompt (fresh session, no memory)
        ├── loop advance → read step-result.txt, transition state machine
        └── cooldown 60s → rate limit protection
```

**Key design:** Each Claude session is stateless. Ralph communicates between iterations via files only (`step-result.txt`, `feedback.md`, `work-summary.md`). The `loop prompt` command reads `state.json` and generates a complete, self-contained prompt for the current step.

### Quick Start
```bash
# Start the loop
tsk ralph

# Or with options
tsk ralph --max 5           # max 5 iterations per task
tsk ralph --task TASK-007   # lock to one specific task
tsk ralph --phase 3         # start from a specific phase
```

### Loop Commands
```bash
tsk loop init [--max N] [--task ID] [--phase N] -o json
tsk loop status -o json
tsk loop prompt
tsk loop advance -o json
tsk loop advance --resume -o json
tsk loop log [--tail N] -o json
tsk loop reset -o json
```

### State Machine

Ralph only runs phases with status `ready` or `in_progress`. Phases with `pending` or `defining` are skipped. Ralph does NOT mark phases as done — user decides when a phase is complete.

```
              ┌──────────┐
              │ ANALYZE  │──── ALL_TASKS_DONE ──► complete (stop, wait for user)
              │          │──── HAS_TASKS ──┐
              └──────────┘                 │
                   ▲                       ▼
                   │               ┌──────────┐
                   │               │IMPLEMENT │
                   │               │          │◄─── REVISE (iteration++)
                   │               └──────────┘
                   │                      │
                   │                      ▼
              ┌──────────┐           ┌──────────┐
              │  (next   │◄── SHIP ──│  REVIEW  │
              │   task)  │           │          │──REVISE──► IMPLEMENT
              └──────────┘           └──────────┘
                                          │
                                     BLOCKED ──► PAUSED (human-input.md)
```

**State transitions in detail:**

| Step | Result | Next Step | Side Effects |
|------|--------|-----------|--------------|
| analyze | `HAS_TASKS` | implement | Picks highest-priority available task, starts it |
| analyze | `ALL_TASKS_DONE` | complete (stop) | Waits for user to add tasks or mark phase done |
| implement | *(auto)* | review | `totalIterations++`, reads `work-summary.md` |
| implement | `BLOCKED` | paused | Human must write `human-input.md` |
| review | `SHIP` | analyze | Marks task done, clears feedback/summary files |
| review | `SHIP` *(but AC unchecked)* | implement | Force-revises with feedback about unchecked AC |
| review | `REVISE` | implement | `iteration++`, writes `feedback.md` |
| review | `REVISE` *(max iterations)* | paused | Human must intervene |

### Phase Lifecycle

Ralph respects phase status as a gate:

| Phase Status | Ralph Behavior |
|---|---|
| `pending` | Skipped — not eligible |
| `defining` | Skipped — not eligible |
| `ready` | Eligible — auto-transitions to `in_progress` when Ralph starts |
| `in_progress` | Eligible — Ralph works on tasks |
| `done` | Skipped — already complete |

User workflow for large phases:
1. Set phase to `ready`
2. Add first batch of tasks
3. Run Ralph — it implements available tasks, stops when done
4. Add more tasks as needed
5. Run Ralph again — it picks up new tasks
6. When satisfied, manually set phase to `done`

### Prompt System

Each step generates a different prompt via `loop prompt`. Prompts are self-contained — they include all context the Claude session needs.

**Analyze prompt** includes:
- Phase name, description, and body
- All tasks in the phase with status
- Instructions to write ONE of: `HAS_TASKS`, `ALL_TASKS_DONE` to `step-result.txt`
- Explicitly told: do NOT create tasks, do NOT decide phase completion

**Implement prompt** includes:
- Task ID, title, spec file path, acceptance criteria
- Current iteration number (e.g., "2/10")
- Previous review feedback from `feedback.md` (if revising)
- Human guidance from `human-input.md` (if resuming from BLOCKED)
- Full workflow: read spec → implement → test → track files → log → commit → write summary
- Can write `BLOCKED` to `step-result.txt` if stuck
- Must commit code changes (uncommitted work is invisible to next iteration)

**Review prompt** includes:
- Task ID, acceptance criteria, modified files list
- Work summary from `work-summary.md`
- Instructions to verify each AC criterion against actual code
- Must update AC checkboxes in task file (`- [ ]` → `- [x]`)
- Must write `SHIP` or `REVISE` to `step-result.txt`
- If `REVISE`: must write specific feedback to `feedback.md`

**Custom prompt appendix:** Users can customize prompts via `tasks/loop/prompts/`:
- `all.append.md` — appended to every step's prompt
- `analyze.append.md` — appended to analyze only
- `implement.append.md` — appended to implement only
- `review.append.md` — appended to review only

### State Files (in `tasks/loop/`)
- `state.json` — current phase, task, step, iteration, maxIterations, status, lockTask
- `step-result.txt` — output keyword from each step (`SHIP`/`REVISE`/`BLOCKED`/etc.)
- `feedback.md` — review feedback for next implement iteration
- `work-summary.md` — what was done this iteration (written by implement step)
- `human-input.md` — human writes guidance here when BLOCKED
- `history.log` — append-only log of all state transitions with timestamps
- `prompts/` — custom prompt appendix files

### state.json Format
```json
{
  "phase": "5",
  "task": "TASK-072",
  "step": "implement",
  "iteration": 0,
  "maxIterations": 10,
  "totalIterations": 15,
  "status": "running",
  "startedAt": "2026-03-19",
  "lockTask": false
}
```

### ralph Orchestrator

The `tsk ralph` command handles:
- **Init/resume logic:** Auto-detects state — inits if missing, resumes if paused with `human-input.md`, re-inits if `--phase` flag passed
- **Progress monitor:** Background spinner showing current step, task, elapsed time, and git file change count
- **Rate limit retry:** Up to 10 retries with 10-minute cooldown when `claude -p` exits non-zero
- **Notifications:** Terminal bell + OSC escape sequences on key events (task shipped, phase complete, errors)
- **Cooldown:** 60-second pause between steps to avoid rate limits

### BLOCKED Recovery
1. Loop pauses → human reads `tasks/loop/feedback.md` for details
2. Human writes guidance to `tasks/loop/human-input.md`
3. Re-run `tsk ralph` → auto-resumes with `--resume`
4. The implement prompt includes human guidance as a dedicated section

### Task Selection Logic

When `HAS_TASKS`, the advance step picks the next task:
1. Prefer `in_progress` tasks (resume over start new)
2. Sort by priority: critical > high > medium > low
3. Filter to current phase only
4. Skip blocked tasks (pending with unmet dependencies)
5. If `lockTask` is set, only work on that specific task

## Rules

1. Always read the spec file before implementing
2. Log progress after each significant step (not every small change)
3. Track all created/modified files with the `files` command
4. Use `--stdin` with `<< 'EOF'` for any multi-line or special-character content
5. Only mark `done` when ALL acceptance criteria are met
6. If blocked, skip to the next available task
7. Do not modify task files directly — always use CLI commands
