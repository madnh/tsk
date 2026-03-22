# Quick Choice: Loop vs Ralph vs Worker

When you want to run tasks, which command should you use?

## TL;DR

- **Just run tasks?** → Use `tsk ralph --phase N`
- **Want full parallelism?** → Use `tsk ralph --phase N` (default)
- **Want sequential execution?** → Use `tsk loop` (old method, still works)
- **Direct worker management?** → Use `tsk worker` (advanced, usually not needed)

## Decision Tree

```
Do you want to run tasks automatically?
│
├─ YES, run in parallel (multiple at once)
│  └─ tsk ralph --phase 1
│     └─ Workers spawn independently, run concurrently
│
├─ YES, run sequentially (one after another)
│  └─ tsk loop init --phase 1
│     └─ Single task execution loop
│
└─ NO, manually manage a single task
   └─ tsk worker run --task TASK-001
      └─ For advanced use only
```

## Command Comparison

| Use Case | Command | When |
|----------|---------|------|
| **Run multiple tasks fast** | `tsk ralph --phase 1` | Most common - 4-5x faster |
| **Run tasks one-by-one** | `tsk loop init --phase 1` | Legacy workflows or debugging |
| **Limit concurrent workers** | `tsk ralph --phase 1 --max-workers 2` | Resource constraints or stability |
| **Monitor progress** | `tsk ralph status` | While ralph is running |
| **View worker status** | `tsk worker status` | Detailed per-task visibility |
| **View worker logs** | `tsk worker logs --task TASK-001` | Debug a specific task |
| **Pause a worker** | `tsk worker kill --task TASK-001` | Kill a stuck task |
| **Resume blocked worker** | `tsk worker resume --task TASK-001` | After providing guidance |
| **Manually run one task** | `tsk worker run --task TASK-001` | Advanced debugging |

## Examples

### Scenario 1: Run Phase 1 (5 tasks)

**What you want:** Run all 5 tasks as fast as possible

```bash
# ✅ CORRECT - Uses parallel execution
tsk ralph --phase 1

# ❌ WRONG - Slow, old way (runs one at a time)
tsk loop init --phase 1
```

**Result:**
- Parallel: ~60 seconds total
- Sequential: ~250 seconds total

### Scenario 2: Limited Resources

**What you want:** Run tasks but don't overwhelm the system (max 2 concurrent)

```bash
# ✅ CORRECT - Respects worker limit
tsk ralph --phase 1 --max-workers 2

# Tasks queue: TASK-001, TASK-002 run first; TASK-003-5 wait
```

### Scenario 3: Debug a Single Task

**What you want:** Run just one task manually for testing

```bash
# ✅ CORRECT - Debug one task
tsk worker run --task TASK-001

# OR use loop (simpler, but slower if you had multiple)
tsk loop init --phase 1
tsk loop advance
```

### Scenario 4: Monitor Running Tasks

**What you want:** Watch progress while ralph is executing

```bash
# Terminal 1
tsk ralph --phase 1

# Terminal 2
watch tsk worker status

# Terminal 3 (optional)
tsk ralph status
tail -f tasks/loop/supervisor.log
```

## Ralph vs Loop (Technical)

| Aspect | Ralph (New) | Loop (Old) |
|--------|------------|-----------|
| **Execution** | Parallel (multiple tasks) | Sequential (one task) |
| **Workers** | Multiple independent processes | Single process loop |
| **Workflow** | Per-type customizable | Fixed (analyze→implement→review) |
| **Branches** | Per-task git worktrees | Single shared branch |
| **Speed** | 4-5x faster with multiple tasks | Baseline (one at a time) |
| **Config** | `ralph.workflows.*` in tsk.yml | `ralph.prompt.*` only |
| **Isolation** | Each task on own branch | All tasks on same branch |
| **Fault Tolerance** | Failed worker doesn't block others | Failure stops loop |

### When to Use Loop (Legacy)

Only use `tsk loop` if you:
1. Have just 1 task
2. Want sequential execution for debugging
3. Prefer the old command structure
4. Need backward compatibility with old scripts

```bash
# These are equivalent for a single task
tsk ralph --phase 1 --max-workers 1  # Modern way
tsk loop init --phase 1               # Old way
```

## Ralph vs Worker (CLI)

| Command | Purpose | Who Uses |
|---------|---------|----------|
| `tsk ralph` | Supervisor - spawns workers, coordinates tasks | You (regular users) |
| `tsk worker` | Individual task executor - runs one task | Supervisor (auto) + Advanced users |

**Normal workflow:**
```
You → tsk ralph --phase 1  (supervisor)
         ↓
      Spawns workers:
      tsk worker run --task TASK-001
      tsk worker run --task TASK-002
      tsk worker run --task TASK-003
```

You don't normally run `tsk worker` directly—ralph does it automatically.

**Only use `tsk worker` directly if:**
- Debugging a single task
- Resuming a blocked worker
- Checking worker status/logs
- Advanced testing

## Common Mistakes

❌ **Mistake 1: Using loop for multiple tasks**
```bash
# SLOW - runs tasks one at a time
tsk loop init --phase 1
```
✅ **Fix: Use ralph instead**
```bash
# FAST - runs tasks in parallel
tsk ralph --phase 1
```

❌ **Mistake 2: Running worker directly for orchestration**
```bash
# WRONG - manually spawning workers
tsk worker run --task TASK-001
tsk worker run --task TASK-002
tsk worker run --task TASK-003
```
✅ **Fix: Let ralph spawn them**
```bash
# RIGHT - ralph orchestrates everything
tsk ralph --phase 1
```

❌ **Mistake 3: Mixing loop and ralph**
```bash
# CONFUSING - two orchestrators fighting
tsk loop init --phase 1
tsk ralph --phase 1  # Conflicts with loop state
```
✅ **Fix: Pick one**
```bash
# Use either loop OR ralph, not both
tsk ralph --phase 1
```

## Migration Guide

If you have scripts using `tsk loop`:

### Old Script (Sequential)
```bash
#!/bin/bash
tsk loop init --phase 1

while [ $(tsk loop status | grep -c "done") -eq 0 ]; do
  tsk loop prompt | claude -p
  tsk loop advance
  sleep 60
done
```

### New Script (Parallel)
```bash
#!/bin/bash
# Much simpler - ralph handles everything
tsk ralph --phase 1
```

**Benefits:**
- Shorter code
- 4-5x faster
- Better isolation
- Automatic worker management

## Recommendations

### For New Projects
- **Always use `tsk ralph`** (newer, faster, better)
- Configure workflows in `tsk.yml` if using different task types
- Use `tsk ralph --phase N` as your main command

### For Existing Projects
- Keep using `tsk loop` if it's working
- Optionally migrate to `tsk ralph` for speedup
- Both systems coexist peacefully (different state files)

### For CI/CD
```yaml
# GitHub Actions, GitLab CI, etc.
jobs:
  tasks:
    steps:
      - run: tsk ralph --phase 1  # Modern approach
```

### For Interactive Use
```bash
# Terminal 1: Run tasks
tsk ralph --phase 1

# Terminal 2: Monitor
watch tsk worker status

# Terminal 3: Debug specific task
tsk worker logs --task TASK-001
```

## Summary

| Scenario | Command |
|----------|---------|
| Run phase (default) | `tsk ralph --phase N` |
| Run phase with limit | `tsk ralph --phase N --max-workers 2` |
| Check supervisor | `tsk ralph status` |
| Check workers | `tsk worker status` |
| View worker logs | `tsk worker logs --task TASK-001` |
| Old sequential way | `tsk loop init --phase N` |
| Debug one task | `tsk worker run --task TASK-001` |
| Resume blocked task | `tsk worker resume --task TASK-001` |

**When in doubt: Use `tsk ralph --phase N`** ✅

---

For detailed information:
- **[Parallel Ralph Docs](PARALLEL_RALPH.md)** — Full feature documentation
- **[Main README](../README.md)** — General usage and commands
