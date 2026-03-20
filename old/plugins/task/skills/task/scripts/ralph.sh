#!/bin/bash

# Ralph — Autonomous AI execution loop
# Resolves project root via CLAUDE_PROJECT_DIR or git, same as task.mjs

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Resolve project root: env var > git > error
if [ -n "$CLAUDE_PROJECT_DIR" ]; then
  ROOT="$CLAUDE_PROJECT_DIR"
elif git rev-parse --show-toplevel >/dev/null 2>&1; then
  ROOT="$(git rev-parse --show-toplevel)"
else
  echo "Error: Cannot determine project root."
  echo "Set CLAUDE_PROJECT_DIR or run from within a git repository."
  exit 1
fi

TASK_CMD="node $SCRIPT_DIR/task.mjs"
MAX_RETRIES=10
RETRY_COUNT=0

cd "$ROOT"

# ─── Notification helper ─────────────────────────────────────────
notify() {
  local title="$1"
  local body="$2"
  printf '\a'
  printf '\033]9;%s\033\\' "$title: $body"
  printf '\033]99;i=ralph:d=0;%s\033\\' "$title: $body"
  echo ""
  echo "🔔 $title: $body"
  echo ""
}

# ─── Progress monitor (runs in background) ───────────────────────
MONITOR_PID=""

start_monitor() {
  local step="$1"
  local task="$2"
  local start_time=$SECONDS
  local spinner='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'

  (
    while true; do
      local elapsed=$(( SECONDS - start_time ))
      local mins=$(( elapsed / 60 ))
      local secs=$(( elapsed % 60 ))
      local time_str=$(printf "%02d:%02d" $mins $secs)

      # Get current file activity
      local changed=$(git diff --name-only 2>/dev/null | wc -l | tr -d ' ')
      local untracked=$(git ls-files --others --exclude-standard 2>/dev/null | wc -l | tr -d ' ')

      # Spinner character
      local i=$(( (elapsed) % ${#spinner} ))
      local char="${spinner:$i:1}"

      # Build status line
      local line="\r\033[K  ${char} \033[1m${step}\033[0m"
      if [ -n "$task" ] && [ "$task" != "null" ]; then
        line+=" ${task}"
      fi
      line+=" \033[90m│\033[0m ⏱ ${time_str}"
      if [ "$changed" -gt 0 ] || [ "$untracked" -gt 0 ]; then
        line+=" \033[90m│\033[0m 📝 ${changed} changed, ${untracked} new"
      fi

      printf "$line" >&2
      sleep 1
    done
  ) &
  MONITOR_PID=$!
}

stop_monitor() {
  if [ -n "$MONITOR_PID" ]; then
    kill $MONITOR_PID 2>/dev/null
    wait $MONITOR_PID 2>/dev/null
    MONITOR_PID=""
    printf "\n" >&2
  fi
}

# Clean up monitor on exit
trap 'stop_monitor; exit' INT TERM EXIT

# ─── Check if --phase was passed (forces re-init) ────────────────
HAS_PHASE_ARG=false
for arg in "$@"; do
  if [ "$arg" = "--phase" ]; then
    HAS_PHASE_ARG=true
    break
  fi
done

# ─── Init or resume ──────────────────────────────────────────────
if [ ! -f tasks/loop/state.json ]; then
  $TASK_CMD loop-init "$@" -o json || exit 1
elif [ "$HAS_PHASE_ARG" = true ]; then
  echo "Re-initializing loop with new phase..."
  $TASK_CMD loop-reset -o json
  $TASK_CMD loop-init "$@" -o json || exit 1
else
  STATUS=$($TASK_CMD loop-status -o json | jq -r '.status')
  if [ "$STATUS" = "paused" ] && [ -f tasks/loop/human-input.md ]; then
    echo "Resuming with human input..."
    $TASK_CMD loop-advance --resume -o json
  elif [ "$STATUS" = "paused" ]; then
    echo "Loop is paused. Write guidance to tasks/loop/human-input.md then re-run."
    exit 1
  elif [ "$STATUS" = "complete" ]; then
    echo "Loop is already complete. Use 'loop-reset' to start over."
    exit 0
  fi
fi

# ─── Main loop ───────────────────────────────────────────────────
while true; do
  # Show current status
  $TASK_CMD loop-status

  # Read current step/task for monitor
  CUR_STEP=$($TASK_CMD loop-status -o json | jq -r '.step')
  CUR_TASK=$($TASK_CMD loop-status -o json | jq -r '.task // empty')

  # Generate prompt for current step
  PROMPT=$($TASK_CMD loop-prompt)
  if [ $? -ne 0 ]; then
    echo "ERROR: Failed to generate prompt"
    exit 1
  fi

  # Start progress monitor
  start_monitor "$CUR_STEP" "$CUR_TASK"

  # Execute with fresh Claude session, capture output
  CLAUDE_OUTPUT=$(echo "$PROMPT" | claude -p --dangerously-skip-permissions 2>&1)
  EXIT_CODE=$?

  # Stop progress monitor, then show claude output
  stop_monitor
  echo "$CLAUDE_OUTPUT"

  if [ $EXIT_CODE -ne 0 ]; then
    RETRY_COUNT=$((RETRY_COUNT + 1))

    STEP=$($TASK_CMD loop-status -o json | jq -r '.step')
    TASK=$($TASK_CMD loop-status -o json | jq -r '.task // "none"')
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  Claude exited with code $EXIT_CODE (step=$STEP, task=$TASK)"
    echo "  Attempt $RETRY_COUNT/$MAX_RETRIES for this step"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    notify "Ralph" "Rate limited (attempt $RETRY_COUNT/$MAX_RETRIES, waiting 10m)"

    if [ $RETRY_COUNT -ge $MAX_RETRIES ]; then
      echo ""
      echo "Max retries reached. Pausing loop."
      echo "State is preserved — re-run ralph.sh to resume when limit resets."
      notify "Ralph" "Max retries reached — loop paused"
      exit 1
    fi

    echo ""
    echo "Waiting 10 minutes for rate limit reset..."
    echo "  (Press Ctrl+C to stop, re-run ralph.sh to resume later)"
    echo ""
    sleep 600
    continue
  fi

  # Success — reset retry counter
  RETRY_COUNT=0

  # Advance state machine
  RESULT=$($TASK_CMD loop-advance -o json)
  if [ $? -ne 0 ]; then
    echo "ERROR: loop-advance failed. State may need manual inspection."
    echo "Run: $TASK_CMD loop-status"
    notify "Ralph" "ERROR: loop-advance failed"
    exit 1
  fi

  STATUS=$(echo "$RESULT" | jq -r '.status')
  ACTION=$(echo "$RESULT" | jq -r '.action')

  # Notify on key events
  case "$ACTION" in
    *"shipped"*|*"Task shipped"*)
      notify "Ralph ✅" "$ACTION" ;;
    *"Phase"*"complete"*|*"PHASE_COMPLETE"*)
      notify "Ralph 🎉" "$ACTION" ;;
    *"SHIP rejected"*)
      notify "Ralph ⚠️" "$ACTION" ;;
  esac

  case "$STATUS" in
    complete)
      notify "Ralph 🏁" "All phases complete!"
      break ;;
    paused)
      notify "Ralph ⏸" "Paused: $(echo "$RESULT" | jq -r '.reason')"
      break ;;
    running)
      echo ""
      echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
      echo "  ✓ Step done — $ACTION"
      echo "  Cooldown 60s. Press Ctrl+C to stop safely."
      echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
      sleep 60
      ;;
  esac
done
