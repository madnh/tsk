# Task Skill

A [Claude Code plugin](https://docs.anthropic.com/en/docs/claude-code/skills) for managing implementation tasks. Tasks are stored as markdown files with frontmatter, tracked through a CLI, and optionally executed by an autonomous loop (Ralph).

## Features

- **Task lifecycle**: `pending → in_progress → review → done` with dependency tracking
- **Priority-based scheduling**: `next` command returns highest-priority unblocked task
- **Phase management**: Group tasks by phase and feature, track progress
- **Autonomous loop (Ralph)**: Analyze → implement → review cycle using `claude -p` sessions
- **JSON output**: All commands support `-o json` for programmatic use

## Installation

### Via marketplace

Add the marketplace, then install:

```bash
claude plugin marketplace add madnh/task-skill
claude plugin install task@madnh-task-skill
```

### Via --plugin-dir

For local development or testing:

```bash
claude --plugin-dir /path/to/task-skill/plugins/task
```

## Usage

Once installed, use the `/task` slash command in Claude Code:

```
/task next              # Find next available task
/task board             # Dashboard overview
/task list --available  # List unblocked tasks
/task progress          # Progress per phase
```

### Task workflow

1. `/task next` - find highest-priority available task
2. `/task start TASK-001` - claim it
3. `/task show TASK-001` - read details and spec
4. Implement the task
5. `/task log TASK-001` - log progress
6. `/task files TASK-001 --add "file1,file2"` - track modified files
7. `/task done TASK-001` - submit for review

### Autonomous loop

Ralph runs tasks unattended in a loop:

```bash
bash <skill-dir>/scripts/ralph.sh
bash <skill-dir>/scripts/ralph.sh --max 5 --task TASK-007
```

See [RALPH.md](plugins/task/skills/task/RALPH.md) for full documentation.

## Project structure

```
task-skill/
├── .claude-plugin/
│   └── marketplace.json      # Marketplace manifest
└── plugins/
    └── task/
        ├── .claude-plugin/
        │   └── plugin.json   # Plugin manifest
        └── skills/
            └── task/
                ├── SKILL.md   # Main skill (loaded by Claude)
                ├── RALPH.md   # Autonomous loop docs (loaded on-demand)
                └── scripts/
                    ├── task.mjs   # CLI tool
                    └── ralph.sh   # Loop orchestrator
```

## Task storage

Tasks live in the consuming project's `tasks/items/` directory as markdown files with YAML frontmatter:

```yaml
---
id: TASK-001
title: Implement database adapter
status: pending
phase: 1
feature: database
priority: high
depends: []
spec: docs/features/database/spec.md
---

## Description
...

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2

## Log
```

## License

MIT
