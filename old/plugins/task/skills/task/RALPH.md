# Autonomous Loop (Ralph)

Ralph is an autonomous AI execution loop that cycles through **analyze → implement → review** to complete pre-defined tasks. It does NOT create tasks or decide what to build — the user defines tasks, Ralph executes them. Each step spawns a fresh `claude -p` session with a generated prompt — state persists only through files, not memory.

## Architecture

```
ralph.sh (bash orchestrator)
  │
  ├── loop-init          → creates state.json
  │
  └── while running:
        ├── loop-status  → display current state
        ├── loop-prompt  → generate step-specific prompt
        ├── claude -p    → execute prompt (fresh session, no memory)
        ├── loop-advance → read step-result.txt, transition state machine
        └── cooldown 60s → rate limit protection
```

**Key design:** Each Claude session is stateless. Ralph communicates between iterations via files only (`step-result.txt`, `feedback.md`, `work-summary.md`). The `loop-prompt` command reads `state.json` and generates a complete, self-contained prompt for the current step.

## Quick Start
```bash
# Start the loop (copies ralph.sh to tasks/loop/ if not present)
bash <skill-dir>/scripts/ralph.sh

# Or with options
bash <skill-dir>/scripts/ralph.sh --max 5           # max 5 iterations per task
bash <skill-dir>/scripts/ralph.sh --task TASK-007   # lock to one specific task
bash <skill-dir>/scripts/ralph.sh --phase 3         # start from a specific phase
```

## Loop Commands
```bash
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs loop-init [--max N] [--task ID] [--phase N] -o json
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs loop-status -o json
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs loop-prompt
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs loop-advance -o json
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs loop-prompt-init -o json
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs loop-log [--tail N] -o json
node ${CLAUDE_SKILL_DIR}/scripts/task.mjs loop-reset -o json
```

## State Machine

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

## Phase Lifecycle

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

## Prompt System

Each step generates a different prompt via `loop-prompt`. Prompts are self-contained — they include all context the Claude session needs.

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

Initialize templates with: `loop-prompt-init`. Write content below the separator line.

## State Files (in `tasks/loop/`)
- `state.json` — current phase, task, step, iteration, maxIterations, status, lockTask
- `step-result.txt` — output keyword from each step (`SHIP`/`REVISE`/`BLOCKED`/etc.)
- `feedback.md` — review feedback for next implement iteration
- `work-summary.md` — what was done this iteration (written by implement step)
- `human-input.md` — human writes guidance here when BLOCKED
- `history.log` — append-only log of all state transitions with timestamps
- `prompts/` — custom prompt appendix files

## state.json Format
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

## ralph.sh Orchestrator

The bash script handles:
- **Init/resume logic:** Auto-detects state — inits if missing, resumes if paused with `human-input.md`, re-inits if `--phase` flag passed
- **Progress monitor:** Background spinner showing current step, task, elapsed time, and git file change count
- **Rate limit retry:** Up to 10 retries with 10-minute cooldown when `claude -p` exits non-zero
- **Notifications:** Terminal bell + OSC escape sequences on key events (task shipped, phase complete, errors)
- **Cooldown:** 60-second pause between steps to avoid rate limits

## BLOCKED Recovery
1. Loop pauses → human reads `tasks/loop/feedback.md` for details
2. Human writes guidance to `tasks/loop/human-input.md`
3. Re-run `bash ralph.sh` → auto-resumes with `--resume`
4. The implement prompt includes human guidance as a dedicated section

## Task Selection Logic

When `HAS_TASKS`, the advance step picks the next task:
1. Prefer `in_progress` tasks (resume over start new)
2. Sort by priority: critical > high > medium > low
3. Filter to current phase only
4. Skip blocked tasks (pending with unmet dependencies)
5. If `lockTask` is set, only work on that specific task