# Parallel Ralph: Concurrent Task Execution with Per-Type Workflows

## Overview

**Parallel Ralph** transforms `tsk ralph` from a sequential, single-task orchestrator into a true parallel execution system. Multiple independent tasks now execute **simultaneously** on isolated git branches, each following a **configurable per-type workflow**.

### Key Features

- **True Parallelism**: All eligible tasks run at the same time (up to `max_workers` limit)
- **Per-Type Workflows**: Each task type (feature, bug, docs, refactor, test, chore) defines its own step sequence
- **Git Worktree Isolation**: Each task executes on its own git branch with automatic cleanup
- **Supervisor Pattern**: Independent worker processes, fault-tolerant (one worker crash doesn't affect others)
- **First-Write-Wins Merging**: Automatic rebase→merge→cleanup handling concurrent modifications
- **State Persistence**: Filesystem-based coordination (no IPC needed)

## Architecture

### Supervisor/Worker Pattern

```
┌─────────────────────────────────────────┐
│        tsk ralph (Supervisor)           │
│  • Polls for pending tasks (every 10s)  │
│  • Spawns worker processes              │
│  • Monitors PIDs                        │
│  • Detects completion                   │
└─────────────────────────────────────────┘
         │         │         │
         ▼         ▼         ▼
    ┌──────┐  ┌──────┐  ┌──────┐
    │Task1 │  │Task2 │  │Task3 │  ...
    │(Git) │  │(Git) │  │(Git) │
    └──────┘  └──────┘  └──────┘
    Worker-1  Worker-2  Worker-3
```

### Process Flow

1. **Supervisor Initialization**
   - Loads phase and checks status
   - Creates `tasks/loop/supervisor.json` with initial state
   - Begins polling for pending tasks

2. **Worker Spawning**
   - For each eligible pending task: `tsk ralph worker run --task TASK-XXX`
   - New subprocess per task
   - Respects `max_workers` limit (queues excess tasks)

3. **Worker Execution**
   - Creates git worktree at `worktrees/TASK-XXX/`
   - Creates branch: `task/TASK-XXX`
   - Executes workflow steps sequentially (from tsk.yml config)
   - Each step: generate prompt → invoke Claude → advance state machine

4. **Completion & Cleanup**
   - When all steps done: rebase onto main
   - Fast-forward merge to main
   - Remove worktree and branch
   - Task marked as done

### State Management

Two separate state files coordinate the system:

**`tasks/loop/supervisor.json`** — Supervisor coordination
```json
{
  "phase": "1",
  "status": "running",
  "startedAt": "2026-03-22T10:30:00Z",
  "workers": [
    {"taskId": "TASK-001", "pid": 12345, "status": "running", "spawnedAt": "..."},
    {"taskId": "TASK-002", "pid": 12346, "status": "running", "spawnedAt": "..."}
  ]
}
```

**`tasks/workers/TASK-XXX/state.json`** — Per-task worker state
```json
{
  "taskId": "TASK-001",
  "taskType": "feature",
  "workflow": ["implement", "review"],
  "stepIndex": 0,
  "iteration": 1,
  "status": "running",
  "worktreePath": "worktrees/TASK-001",
  "branchName": "task/TASK-001"
}
```

## Per-Type Workflows

Each task type follows a configurable workflow defined in `tsk.yml`:

### Built-In Workflows

```yaml
workflows:
  feature:  [implement, review]        # Code feature: implement then review
  bug:      [analyze, implement, review]  # Bug fix: analyze, implement, review
  refactor: [analyze, implement, review]  # Refactoring: analyze, implement, review
  test:     [implement, review]        # Test: implement then review
  chore:    [implement, review]        # Chore: implement then review
  docs:     [write, review]            # Documentation: write then review (no code testing)
  plan:     [brainstorm, spec, review] # Planning: brainstorm, spec, review
```

### Step Descriptions

| Step | Purpose | Output |
|------|---------|--------|
| **analyze** | Understand requirements, break down tasks | Analysis document |
| **brainstorm** | Explore approaches and design options | Brainstorm notes |
| **spec** | Formalize design into specification | Specification document |
| **implement** | Write code/content | Code changes + commit |
| **write** | Write documentation content | Documentation |
| **test** | Create tests | Test code + results |
| **review** | Check against acceptance criteria | SHIP or REVISE decision |

### Step Result Protocol

Workers communicate step results via `tasks/workers/TASK-XXX/step-result.txt`:

- **`SHIP`** (review step) → Task complete, ready to merge
- **`REVISE`** (review step) → Iterate (increment iteration count)
- **`BLOCKED`** (any step) → Worker paused, waiting for human input
- **Empty/Success** (other steps) → Advance to next step

## Configuration

Add these sections to `tsk.yml`:

```yaml
ralph:
  max_workers: 0                        # 0 = unlimited, N = max concurrent workers
  supervisor_poll: 10                   # seconds between polling for new tasks
  max_iterations: 10                    # max revision cycles per task
  cooldown: 60                          # seconds between steps (rate limiting)

  workflows:
    feature:  [implement, review]
    bug:      [analyze, implement, review]
    docs:     [write, review]
    # ... add your custom workflows

  prompt:
    all: |
      # Instructions appended to ALL steps
    analyze: |
      # Instructions for analyze step
    brainstorm: |
      # Instructions for brainstorm step
    spec: |
      # Instructions for spec step
    implement: |
      # Instructions for implement step
    write: |
      # Instructions for write step (docs)
    test: |
      # Instructions for test step
    review: |
      # Instructions for review step
```

### Configuration Defaults

If not specified in `tsk.yml`:
- `max_workers`: 0 (unlimited)
- `supervisor_poll`: 10 seconds
- `max_iterations`: 10
- Default workflows: feature/bug/chore use [implement, review]
- Default prompts: Reasonable defaults for each step type

## Directory Structure

```
your-project/
├── tsk.yml
└── tasks/
    ├── items/
    │   ├── TASK-001.md
    │   ├── TASK-002.md
    │   └── ...
    ├── phases/
    │   ├── phase-1.md
    │   └── ...
    ├── workers/                        # Created during execution
    │   ├── TASK-001/
    │   │   ├── state.json              # Worker state
    │   │   ├── step-result.txt         # Current step result
    │   │   ├── work-summary.md         # Summary of work done
    │   │   ├── feedback.md             # Review feedback (if iterating)
    │   │   ├── human-input.md          # Guidance (if blocked)
    │   │   └── history.log             # Activity log
    │   └── ...
    └── loop/
        ├── supervisor.json             # Supervisor state
        ├── supervisor.log              # Supervisor activity log
        └── state.json                  # (Unused, kept for compatibility)

worktrees/                             # Created during execution
├── TASK-001/
│   ├── (branch: task/TASK-001)
│   └── (all project files)
└── ...
```

## CLI Reference

### Supervisor Commands

```bash
# Run supervisor for a phase
tsk ralph run --phase 1

# Run with worker limit (instead of unlimited)
tsk ralph run --phase 1 --max-workers 3

# Check supervisor status
tsk ralph status
```

### Worker Commands

These are typically invoked by the supervisor, but can be used manually:

```bash
# Start a worker (normally spawned by supervisor)
tsk ralph worker run --task TASK-001

# Show all worker statuses
tsk ralph worker status

# Show status of specific worker
tsk ralph worker status --task TASK-001

# View worker logs
tsk ralph worker logs --task TASK-001

# Resume a blocked worker (after providing human-input.md)
tsk ralph worker resume --task TASK-001

# Kill a worker gracefully (SIGTERM)
tsk ralph worker kill --task TASK-001
```

## Usage Examples

### Example 1: Basic Parallel Execution

Run 5 tasks simultaneously with unlimited workers:

```bash
$ tsk ralph run --phase 1
Supervisor starting...
[TASK-001] Worker spawned (PID 12345)
[TASK-002] Worker spawned (PID 12346)
[TASK-003] Worker spawned (PID 12347)
[TASK-004] Worker spawned (PID 12348)
[TASK-005] Worker spawned (PID 12349)

# In another terminal
$ watch tsk ralph worker status
```

Expected timeline:
- T=0s: All 5 workers start
- T=10s: All executing step 1 (implement/write/analyze)
- T=30s: All executing step 2 (review)
- T=50s: All merging to main and cleaning up
- T=60s: All done (vs 300s sequential)

### Example 2: Limited Workers (Queuing)

Run 5 tasks with only 2 concurrent workers:

```bash
$ tsk ralph run --phase 1 --max-workers 2
Supervisor starting...
[TASK-001] Worker spawned (PID 12345)
[TASK-002] Worker spawned (PID 12346)
[TASK-003] Queued (waiting for worker availability)
[TASK-004] Queued
[TASK-005] Queued

# Monitor progress
$ watch tsk ralph worker status
```

Timeline:
- T=0-50s: TASK-001, TASK-002 executing
- T=50-100s: TASK-003, TASK-004 executing
- T=100-150s: TASK-005 executing
- T=150s: All done (sequential batching)

### Example 3: Per-Type Workflow (Mixed Types)

Phase with different task types:

```yaml
# tsk.yml
workflows:
  feature: [implement, review]      # 2 steps
  docs:    [write, review]          # Different steps!
  bug:     [analyze, implement, review]  # 3 steps
```

Tasks in phase:
- TASK-001 (feature): [implement, review]
- TASK-002 (docs): [write, review]
- TASK-003 (bug): [analyze, implement, review]

Execution:
```
T=10s:   TASK-001/002/003 all step 1 (implement/write/analyze respectively)
T=30s:   TASK-001 step 2 (review), TASK-003 step 2 (implement), TASK-002 step 2 (review)
T=50s:   TASK-003 step 3 (review), others done
T=60s:   All done (same parallel timeline, but different internal steps)
```

### Example 4: Monitor and Manage

```bash
# Terminal 1: Run supervisor
$ tsk ralph run --phase 1

# Terminal 2: Monitor progress
$ watch tsk ralph worker status

# Terminal 3: Check supervisor
$ tsk ralph status

# Terminal 4: View specific worker logs
$ tsk ralph worker logs --task TASK-001
```

## Performance Comparison

### Sequential Execution (Old Ralph)

- Task 1: analyze (10s) → implement (30s) → review (10s) = 50s
- Task 2: analyze (10s) → implement (30s) → review (10s) = 50s
- Task 3: analyze (10s) → implement (30s) → review (10s) = 50s
- **Total: 150 seconds** (serial execution)

### Parallel Execution (New Ralph, unlimited workers)

- T=0-10s: All tasks analyzing/implementing/writing
- T=10-40s: All tasks in middle steps
- T=40-50s: All tasks in review
- T=50-60s: All tasks merging + cleanup
- **Total: 60 seconds** (2.5x faster)

### Speedup Formula

With N tasks and unlimited workers:
```
Sequential time: N × (sum of step times)
Parallel time:   sum of step times (+ merge/cleanup time)
Speedup:         N × (roughly N:1 ratio)
```

Actual speedup depends on:
- Number of eligible tasks
- Task complexity and step times
- `max_workers` limit (queuing reduces speedup)
- Merge conflicts (rebasing may take extra time)

## Conflict Resolution

When multiple tasks modify the same files in parallel:

1. **Before Merge**: Each worker rebases onto main
   ```bash
   git -C worktrees/TASK-001 fetch origin main
   git -C worktrees/TASK-001 rebase origin/main
   ```

2. **Conflict Handling**: If rebase conflicts occur
   - Worker pauses with `status: blocked`
   - Writes conflict details to `feedback.md`
   - Can be resolved manually or skipped via `resume`

3. **Fast-Forward Merge**: Non-conflicting tasks merge cleanly
   ```bash
   git merge --ff-only task/TASK-001
   ```

## Troubleshooting

### Supervisor Won't Start

Check environment:
```bash
./tsk doctor        # Verify git, claude, worktree support
git status          # Verify git repo initialized
cat tsk.yml | grep -A 5 "^ralph:"  # Check config
cat tasks/phases/phase-1.md | grep "^status:"  # Check phase is ready
```

### Worker Blocked

View feedback and provide guidance:
```bash
cat tasks/workers/TASK-001/feedback.md

# Write guidance
echo "Here's how to proceed..." > tasks/workers/TASK-001/human-input.md

# Resume
tsk ralph worker resume --task TASK-001
```

### Check Logs

```bash
# Supervisor log
tail -100 tasks/loop/supervisor.log

# Specific worker log
tsk ralph worker logs --task TASK-001 | tail -50

# Git activity in worktree
cd worktrees/TASK-001 && git log --oneline -10
```

### Merge Conflicts

Workers detect conflicts during rebase:
```bash
# Check state
cat tasks/workers/TASK-001/state.json | jq .status

# View conflict details
cat tasks/workers/TASK-001/feedback.md

# Manually resolve (if needed)
cd worktrees/TASK-001
git status  # see conflicts
# edit files to resolve
git add .
git rebase --continue

# Resume worker
tsk ralph worker resume --task TASK-001
```

### Kill a Stuck Worker

Graceful termination:
```bash
tsk ralph worker kill --task TASK-002
```

This sends SIGTERM and waits for cleanup. The task state remains in `tasks/workers/TASK-002/` for inspection.

## Advanced Configuration

### Custom Workflows

Define workflows for your task types:

```yaml
workflows:
  security: [analyze, implement, test, review]  # extra test step
  migration: [spec, implement, verify, review]  # verify instead of test
  refactor: [analyze, implement, review]
```

### Custom Prompts per Step

Tailor Claude's behavior for each step:

```yaml
prompt:
  all: |
    # Applied to EVERY step
    You are working in a feature branch. Commit changes frequently.

  analyze: |
    # For analyze step only
    Focus on understanding the requirements and breaking down the work.

  implement: |
    # For implement step
    Write production-quality code with tests.
    Use the existing project style and patterns.

  review: |
    # For review step
    Verify the implementation matches all acceptance criteria.
    If any AC fails, respond with REVISE.
    If all AC pass, respond with SHIP.
```

### Rate Limiting & Cooldowns

Control API rate limits:

```yaml
ralph:
  cooldown: 60                    # seconds between steps (default: 60)
  retry_max: 10                   # max retries on rate limit
  retry_wait: 600                 # seconds to wait on retry
```

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Parallel Ralph Execution

on:
  schedule:
    - cron: '0 9 * * MON'  # Weekly on Monday at 9am

jobs:
  execute-tasks:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Download tsk
        run: |
          curl -Lo tsk https://github.com/madnh/tsk/releases/latest/download/tsk_linux_amd64.tar.gz
          tar xzf tsk_*.tar.gz && chmod +x tsk

      - name: Set up Claude CLI
        run: |
          npm install -g @anthropic-ai/claude

      - name: Run parallel tasks
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          ./tsk ralph run --phase 1

      - name: Push changes
        if: success()
        run: |
          git config --global user.name "Claude"
          git config --global user.email "claude@anthropic.com"
          git add -A
          git commit -m "Tasks completed by Ralph" || true
          git push
```

## Best Practices

### Task Design

1. **Keep Tasks Independent**: Avoid dependencies when possible for maximum parallelism
2. **Clear Acceptance Criteria**: Each AC should be independently verifiable
3. **Reasonable Scope**: Each task should complete in 30-60 seconds per step
4. **Type Classification**: Use task types that match your workflow definitions

### Configuration

1. **Start Conservative**: Use `max_workers: 2-3` initially, increase based on stability
2. **Define Workflows**: Explicitly list workflows for your task types
3. **Custom Prompts**: Add domain-specific instructions to improve Claude's output
4. **Monitor Logs**: Check logs frequently when debugging issues

### Execution

1. **Prepare the Phase**: Ensure phase status is `ready` before running
2. **Monitor Progress**: Watch supervisor and worker status in separate terminals
3. **Handle Failures**: Check blocked workers early and provide guidance
4. **Review Results**: Inspect git log and task states after completion

## Comparison with Sequential Ralph

| Aspect | Sequential Ralph | Parallel Ralph |
|--------|-----------------|----------------|
| **Execution** | One task at a time | Multiple simultaneous tasks |
| **Workflow Steps** | Fixed (analyze → implement → review) | Per-type customizable |
| **Git Isolation** | Single branch | Per-task worktrees |
| **Worker Pattern** | Single process loop | Multiple independent workers |
| **Fault Tolerance** | One failure stops all | Failed workers don't block others |
| **Performance** | 5 tasks = 250 seconds | 5 tasks = 50 seconds |
| **Configuration** | Global prompt settings | Per-type workflows + per-step prompts |

## Backward Compatibility

- Old `tsk loop` commands work unchanged
- Existing task files require no modifications
- Old phases/tasks automatically compatible
- `tasks/loop/state.json` untouched (separate from supervisor.json)
- Optional feature: Enable only when needed

---

**For examples and testing**, see the [Astro Blog Test Project](https://github.com/madnh/tsk/tree/main/examples/astro-blog).

**For troubleshooting**, check logs at:
- Supervisor: `tasks/loop/supervisor.log`
- Worker: `tasks/workers/TASK-XXX/history.log`
- Git: `worktrees/TASK-XXX/.git/`
