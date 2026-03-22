# tsk

Task management CLI for structured project execution with an autonomous AI loop.

Single binary, no runtime dependencies. Download and run from anywhere.

## Install as Claude Code Skill

Use `tsk` as a [Claude Code skill](https://github.com/madnh/tsk-skill) so Claude can manage tasks directly:

```
/plugin marketplace add madnh/tsk-skill
/plugin install tsk@madnh-tsk-skill
```

Claude will auto-invoke when you say "task", "next task", "what to do", "progress", "board".

## Install CLI

### Download binary (recommended)

Download the latest release from [GitHub Releases](https://github.com/madnh/tsk/releases):

```bash
# Linux (amd64)
curl -Lo tsk https://github.com/madnh/tsk/releases/latest/download/tsk_$(curl -s https://api.github.com/repos/madnh/tsk/releases/latest | grep tag_name | cut -d '"' -f4 | sed 's/v//')_linux_amd64.tar.gz
tar xzf tsk_*.tar.gz
chmod +x tsk
sudo mv tsk /usr/local/bin/

# macOS (Apple Silicon)
curl -Lo tsk.tar.gz https://github.com/madnh/tsk/releases/latest/download/tsk_$(curl -s https://api.github.com/repos/madnh/tsk/releases/latest | grep tag_name | cut -d '"' -f4 | sed 's/v//')_darwin_arm64.tar.gz
tar xzf tsk.tar.gz
chmod +x tsk
sudo mv tsk /usr/local/bin/
```

Or manually: go to [Releases](https://github.com/madnh/tsk/releases), download the archive for your OS/arch, extract, and put `tsk` in your PATH.

### Keep updated

`tsk` includes built-in auto-update support. Check for and install updates:

```bash
# Check for updates without installing
tsk update --check

# Update to the latest version (with confirmation)
tsk update

# Update without confirmation prompt
tsk update --yes

# Install a specific version
tsk update --version v0.3.0
```

The update feature:
- Downloads the binary for your OS/architecture
- Verifies checksums against the official release
- Atomically replaces your current binary
- Shows progress during download

### Build from source

```bash
go build -o tsk .
sudo mv tsk /usr/local/bin/
```

## Quick Start

### Automated Execution (Recommended)

```bash
# Initialize project
tsk init

# Create tasks and phase
tsk create --title "Feature 1" --phase 1 --priority high
tsk create --title "Feature 2" --phase 1 --priority high

# Run tasks in parallel (fast!)
tsk ralph --phase 1

# Monitor progress in another terminal
tsk worker status
```

**→ [Confused about loop vs ralph vs worker?](docs/QUICK_CHOICE.md)** See the quick choice guide.

### Manual Task Management

```bash
# Work on tasks individually
tsk next                    # find next available task
tsk start TASK-001          # claim it
tsk log TASK-001 --stdin << 'EOF'
- Implemented feature
- Added tests
EOF
tsk files TASK-001 --add "feature.go,feature_test.go"
tsk done TASK-001           # submit for review

# Review
tsk approve TASK-001
tsk reject TASK-001 --message "Need more tests"

# Dashboard
tsk board
tsk progress
```

## Commands

### Task Lifecycle

```
pending ──► in_progress ──► review ──► done
                 ▲                      │
                 └──── (reject) ────────┘
```

| Command | Description |
|---------|-------------|
| `tsk create` | Create a new task (`--title`, `--phase`, `--feature` required) |
| `tsk start <id>` | Start a task (pending → in_progress) |
| `tsk done <id>` | Submit for review (in_progress → review) |
| `tsk approve <id>` | Approve (review → done) |
| `tsk reject <id>` | Reject with feedback (review → in_progress) |
| `tsk log <id>` | Append log entry (`--message` or `--stdin`) |
| `tsk files <id>` | Track modified files (`--add "f1,f2"`) |
| `tsk edit <id>` | Update metadata (`--title`, `--phase`, `--priority`, `--depends`, etc.) |
| `tsk delete <id>` | Delete a task (blocked if others depend on it) |

### View

| Command | Description |
|---------|-------------|
| `tsk board` | Dashboard with phases, active/review/blocked tasks |
| `tsk list` | List tasks (`--phase`, `--status`, `--feature`, `--available`) |
| `tsk show <id>` | Full task detail with body |
| `tsk next` | Suggest next available task by priority |
| `tsk progress` | Progress bars per phase |
| `tsk deps <id>` | Dependency tree (`--reverse` for dependents) |

### Phases

| Command | Description |
|---------|-------------|
| `tsk phase` | List all phases |
| `tsk phase <N>` | Show phase detail with tasks |
| `tsk phase <N> --status ready` | Update phase status |
| `tsk phase log <N>` | Add log entry to phase |
| `tsk phase update-body <N>` | Replace phase body (`--stdin`) |

Phase statuses: `pending` → `defining` → `ready` → `in_progress` → `done`

### Autonomous Loop (Ralph)

Ralph orchestrates task completion automatically. It can run in two modes:

#### Sequential Mode (Classic)

Single-task linear execution with fixed steps (analyze → implement → review):

```bash
# Run the full loop
tsk ralph --phase 1 --max 5

# Or manage manually
tsk loop init --phase 1
tsk loop status
tsk loop prompt       # generate prompt for current step
tsk loop advance      # advance state machine
tsk loop log          # view history
tsk loop reset        # clear state
```

#### Parallel Mode (New)

Multiple tasks execute **simultaneously** on isolated git branches, each following a **per-type workflow**:

```bash
# Run supervisor for parallel execution
tsk ralph --phase 1                    # Unlimited concurrent workers
tsk ralph --phase 1 --max-workers 3    # Limit to 3 concurrent workers

# Monitor progress
tsk ralph status                       # Check supervisor status
tsk worker status                      # List all workers
tsk worker logs --task TASK-001        # View specific worker logs
```

**Key Features:**
- **True Parallelism**: All eligible tasks run simultaneously (up to limit)
- **Per-Type Workflows**: Each task type defines its own step sequence (feature, bug, docs, refactor, etc.)
- **Git Worktree Isolation**: Each task executes on its own branch with automatic cleanup
- **Fault Tolerance**: Failed workers don't block others; independent recovery

**Documentation:**
- **[PARALLEL_RALPH.md](docs/PARALLEL_RALPH.md)** — Detailed feature docs, config, examples
- **[QUICK_CHOICE.md](docs/QUICK_CHOICE.md)** — When to use loop vs ralph vs worker

#### State Machine

```
            ┌──────────┐
            │ ANALYZE  │──── ALL_TASKS_DONE ──► complete
            │          │──── HAS_TASKS ──┐
            └──────────┘                 │
                 ▲                       ▼
                 │               ┌──────────┐
                 │               │IMPLEMENT │
                 │               │          │◄── REVISE (iteration++)
                 │               └──────────┘
                 │                      │
                 │                      ▼
            ┌──────────┐         ┌──────────┐
            │  (next   │◄─ SHIP ─│  REVIEW  │
            │   task)  │         │          │── REVISE ──► IMPLEMENT
            └──────────┘         └──────────┘
                                       │
                                  BLOCKED ──► PAUSED
```

#### BLOCKED Recovery

1. Loop pauses → check `tasks/loop/feedback.md`
2. Write guidance to `tasks/loop/human-input.md`
3. Re-run `tsk ralph` → auto-resumes

### Other

| Command | Description |
|---------|-------------|
| `tsk init` | Create `tsk.yml` and directory structure |
| `tsk doctor` | Check environment (tsk.yml, git, claude, paths) |
| `tsk update` | Update to latest version (`--check` to check only, `--yes` to skip confirmation) |
| `tsk version` | Show version, commit, and build date |

## Output Format

All commands support `-o json` for machine-readable output:

```bash
tsk list -o json
tsk board -o json
tsk show TASK-001 -o json
```

## Configuration (`tsk.yml`)

Generated by `tsk init`:

```yaml
project:
  name: "My Project"

ralph:
  max_iterations: 10        # max implement/review cycles per task
  cooldown: 60               # seconds between steps
  retry_max: 10              # retries on rate limit
  retry_wait: 600            # seconds to wait on retry
  claude:
    command: "claude"
    args: ["-p", "--dangerously-skip-permissions"]
  prompt:
    all: |
      # Instructions appended to ALL steps
    analyze: |
      # Instructions for analyze step
    implement: |
      # Instructions for implement step
    review: |
      # Instructions for review step

task:
  default_priority: "medium"

doctor:
  required_tools: ["git", "claude"]
```

Prompt lines starting with `#` are ignored. Add real instructions below the comments.

## Root Detection

`tsk` finds the project root in this order:

1. `--root-dir` flag
2. `TSK_ROOT_DIR` environment variable
3. Walk up from cwd looking for `tsk.yml`
4. `git rev-parse --show-toplevel`
5. Current directory

## Task File Format

Tasks are stored as markdown in `tasks/items/TASK-NNN.md`:

```markdown
---
id: TASK-001
title: Implement user auth
status: pending
phase: 1
feature: auth
priority: high
depends: []
spec: docs/features/auth/spec.md
files: []
created: 2026-03-20
started:
completed:
---

## Description

Add user authentication with JWT tokens.

## Acceptance Criteria

- [ ] Login endpoint returns JWT
- [ ] Middleware validates tokens
- [ ] Unit tests for auth flow

## Log

### 2026-03-20 - AI Agent
Started implementation...
```

## Project Structure

```
your-project/
├── tsk.yml
└── tasks/
    ├── items/          # TASK-001.md, TASK-002.md, ...
    ├── phases/         # phase-1.md, phase-2.md, ...
    └── loop/           # state.json, history.log, ...
```

## Development

### Prerequisites

- Go 1.22+

### Source layout

```
tsk/
├── main.go
├── go.mod
├── cmd/                    # CLI commands (cobra)
│   ├── root.go             # Root command, global flags (-o, --root-dir)
│   ├── init.go             # tsk init
│   ├── create.go           # tsk create
│   ├── list.go             # tsk list
│   ├── show.go             # tsk show
│   ├── board.go            # tsk board
│   ├── phase.go            # tsk phase + subcommands (log, update-body)
│   ├── loop.go             # tsk loop + subcommands (init, status, prompt, advance, log, reset)
│   ├── ralph.go            # tsk ralph (autonomous orchestrator)
│   ├── doctor.go           # tsk doctor
│   └── ...                 # start, done, approve, reject, log, files, edit, delete, next, progress, deps
├── internal/
│   ├── config/config.go    # tsk.yml loading + root detection
│   ├── model/              # Task, Phase, LoopState structs
│   ├── store/              # File CRUD (frontmatter parser, task/phase/loop stores)
│   ├── engine/             # Business logic (dependencies, blocking, state machine)
│   ├── prompt/prompt.go    # Generate analyze/implement/review prompts
│   ├── output/             # JSON/pretty output formatting
│   └── embedded/           # go:embed default tsk.yml
└── .goreleaser.yml
```

### Build & run

```bash
go build -o tsk .
./tsk --help
```

### Run checks

```bash
go vet ./...
```

### Release

Releases are automated via GitHub Actions + [GoReleaser](https://goreleaser.com/). Tag a version to trigger:

```bash
git tag v0.2.0
git push origin v0.2.0
```

This builds binaries for linux/darwin/windows (amd64 + arm64) and publishes them to [GitHub Releases](https://github.com/madnh/tsk/releases).

### Key design decisions

- **Frontmatter parser**: Custom line-by-line parser (not `yaml.v3`) to match the original Node.js format exactly. Arrays as `[item1, item2]` on a single line.
- **Prompts in config**: Custom prompts live in `tsk.yml` under `ralph.prompt.<step>`, not separate files. Lines starting with `#` are stripped.
- **Root detection**: 5-level priority chain (flag → env → tsk.yml walk-up → git → cwd) so `tsk` works from any subdirectory.
- **Ralph in Go**: Replaces the bash orchestrator. Uses goroutines for progress monitor, `exec.CommandContext` for claude invocation, context-based cancellation for clean shutdown.
- **No test framework**: Standard library only. `go vet` for static checks.
