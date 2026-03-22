package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/config"
	"github.com/madnh/tsk/internal/engine"
	"github.com/madnh/tsk/internal/model"
	"github.com/madnh/tsk/internal/output"
	"github.com/madnh/tsk/internal/prompt"
)

var ralphCmd = &cobra.Command{
	Use:   "ralph",
	Short: "Run the autonomous execution loop",
	Run: func(cmd *cobra.Command, args []string) {
		phaseFlag, _ := cmd.Flags().GetString("phase")
		maxFlag, _ := cmd.Flags().GetInt("max")
		taskFlag, _ := cmd.Flags().GetString("task")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle signals
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Fprintln(os.Stderr, "\nShutting down...")
			cancel()
		}()

		// Init or resume
		if !loopStore.StateExists() {
			initLoop(phaseFlag, maxFlag, taskFlag)
		} else if phaseFlag != "" {
			fmt.Println("Re-initializing loop with new phase...")
			loopStore.Reset()
			loopStore.Log("RESET")
			initLoop(phaseFlag, maxFlag, taskFlag)
		} else {
			state, err := loopStore.ReadState()
			if err != nil {
				output.Fail(fmt.Sprintf("Failed to read state: %v", err))
			}
			if state.Status == "paused" && loopStore.FileExists("human-input.md") {
				fmt.Println("Resuming with human input...")
				eng := &engine.LoopEngine{
					LoopStore: loopStore, TaskStore: taskStore, PhaseStore: phaseStore,
				}
				eng.Advance(state, true)
			} else if state.Status == "paused" {
				fmt.Println("Loop is paused. Write guidance to tasks/loop/human-input.md then re-run.")
				os.Exit(1)
			} else if state.Status == "complete" {
				fmt.Println("Loop is already complete. Use 'tsk loop reset' to start over.")
				os.Exit(0)
			}
		}

		maxRetries := config.GetRetryMax()
		retryCount := 0
		retryWait := time.Duration(config.GetRetryWait()) * time.Second
		cooldown := time.Duration(config.GetCooldown()) * time.Second

		// Main loop
		for {
			if ctx.Err() != nil {
				break
			}

			state, _ := loopStore.ReadState()
			if state == nil || state.Status != model.LoopRunning {
				break
			}

			// Show status
			printLoopStatus(state)

			// Generate prompt
			gen := &prompt.Generator{
				LoopStore: loopStore, TaskStore: taskStore,
				PhaseStore: phaseStore, RootDir: cfg.Root,
			}
			promptText, err := gen.Generate(state)
			if err != nil {
				fmt.Printf("ERROR: Failed to generate prompt: %v\n", err)
				os.Exit(1)
			}

			// Record step start
			state.StepStartedAt = time.Now().Format(time.RFC3339)
			loopStore.WriteState(state)
			logEntry := fmt.Sprintf("START step=%s phase=%s", state.Step, state.Phase)
			if state.Task != "" {
				logEntry += fmt.Sprintf(" task=%s", state.Task)
			}
			loopStore.Log(logEntry)

			// Start progress monitor
			var monitorStop context.CancelFunc
			var monitorCtx context.Context
			monitorCtx, monitorStop = context.WithCancel(ctx)
			var monitorWg sync.WaitGroup
			monitorWg.Add(1)
			go func() {
				defer monitorWg.Done()
				runMonitor(monitorCtx, state.Step, state.Task)
			}()

			// Execute claude
			claudeOutput, exitCode := executeClaudeCmd(ctx, promptText)

			// Stop monitor
			monitorStop()
			monitorWg.Wait()
			fmt.Fprintln(os.Stderr)

			if claudeOutput != "" {
				fmt.Println(claudeOutput)
			}

			if exitCode != 0 {
				retryCount++
				fmt.Println()
				fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				fmt.Printf("  Claude exited with code %d (step=%s, task=%s)\n", exitCode, state.Step, state.Task)
				fmt.Printf("  Attempt %d/%d for this step\n", retryCount, maxRetries)
				fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				notify("Ralph", fmt.Sprintf("Rate limited (attempt %d/%d, waiting 10m)", retryCount, maxRetries))

				if retryCount >= maxRetries {
					fmt.Println()
					fmt.Println("Max retries reached. Pausing loop.")
					notify("Ralph", "Max retries reached — loop paused")
					os.Exit(1)
				}

				fmt.Println()
				fmt.Printf("Waiting %v for rate limit reset...\n", retryWait)
				fmt.Println("  (Press Ctrl+C to stop, re-run ralph to resume later)")
				select {
				case <-time.After(retryWait):
				case <-ctx.Done():
					os.Exit(0)
				}
				continue
			}

			retryCount = 0

			// Advance state machine
			eng := &engine.LoopEngine{
				LoopStore: loopStore, TaskStore: taskStore, PhaseStore: phaseStore,
			}
			state, _ = loopStore.ReadState()
			result, err := eng.Advance(state, false)
			if err != nil {
				fmt.Printf("ERROR: loop advance failed: %v\n", err)
				notify("Ralph", "ERROR: loop advance failed")
				os.Exit(1)
			}

			// Notify on key events
			switch {
			case strings.Contains(result.Action, "shipped") || strings.Contains(result.Action, "Task shipped"):
				notify("Ralph ✅", result.Action)
			case strings.Contains(result.Action, "Phase") && strings.Contains(result.Action, "complete"):
				notify("Ralph 🎉", result.Action)
			case strings.Contains(result.Action, "SHIP rejected"):
				notify("Ralph ⚠️", result.Action)
			}

			// Auto push if configured
			if config.GetAutoPush() {
				pushOut, pushErr := exec.Command("git", "push").CombinedOutput()
				if pushErr != nil {
					fmt.Printf("⚠ Auto-push failed: %s\n", strings.TrimSpace(string(pushOut)))
					loopStore.Log(fmt.Sprintf("PUSH_FAILED: %s", strings.TrimSpace(string(pushOut))))
				} else {
					fmt.Println("  ↑ Auto-pushed to remote")
					loopStore.Log("PUSH_OK")
				}
			}

			switch result.Status {
			case "complete":
				notify("Ralph 🏁", "All phases complete!")
				return
			case "paused":
				notify("Ralph ⏸", fmt.Sprintf("Paused: %s", result.Reason))
				return
			case "running":
				fmt.Println()
				fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				fmt.Printf("  ✓ Step done — %s\n", result.Action)
				fmt.Printf("  Cooldown %v. Press Ctrl+C to stop safely.\n", cooldown)
				fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				select {
				case <-time.After(cooldown):
				case <-ctx.Done():
					return
				}
			}
		}
	},
}

func initLoop(phase string, max int, task string) {
	loopStore.EnsureDir()
	phases, _ := phaseStore.All()
	if len(phases) == 0 {
		output.Fail("No phases found.")
	}

	if max == 0 {
		max = 10
	}

	var currentPhase *model.Phase
	if phase != "" {
		for _, p := range phases {
			if p.Num == phase {
				currentPhase = p
				break
			}
		}
		if currentPhase == nil {
			output.Fail(fmt.Sprintf("Phase %s not found.", phase))
		}
		if !engine.IsPhaseRunnable(currentPhase) {
			output.Fail(fmt.Sprintf("Phase %s is '%s'. Only 'ready' or 'in_progress' phases can be run.",
				phase, currentPhase.Status))
		}
	} else {
		for _, p := range phases {
			if engine.IsPhaseRunnable(p) {
				currentPhase = p
				break
			}
		}
		if currentPhase == nil {
			output.Fail("No runnable phases.")
		}
	}

	if currentPhase.Status == "ready" {
		currentPhase.Status = "in_progress"
		phaseStore.Write(currentPhase)
	}

	state := &model.LoopState{
		Phase:         currentPhase.Num,
		Task:          task,
		Step:          model.StepAnalyze,
		MaxIterations: max,
		Status:        model.LoopRunning,
		StartedAt:     todayStr(),
		LockTask:      task != "",
	}
	loopStore.WriteState(state)
	loopStore.Log(fmt.Sprintf("INIT phase=%s max=%d", currentPhase.Num, max))
}

func executeClaudeCmd(ctx context.Context, promptText string) (string, int) {
	claudeCmd := config.GetClaudeCommand()
	claudeArgs := config.GetClaudeArgs()

	cmd := exec.CommandContext(ctx, claudeCmd, claudeArgs...)
	cmd.Stdin = strings.NewReader(promptText)
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return string(out), exitCode
}

func runMonitor(ctx context.Context, step, task string) {
	spinner := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	start := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			elapsed := int(time.Since(start).Seconds())
			mins := elapsed / 60
			secs := elapsed % 60
			timeStr := fmt.Sprintf("%02d:%02d", mins, secs)

			idx := elapsed % len(spinner)
			char := string(spinner[idx])

			line := fmt.Sprintf("\r\033[K  %s \033[1m%s\033[0m", char, step)
			if task != "" && task != "null" {
				line += " " + task
			}
			line += fmt.Sprintf(" \033[90m│\033[0m ⏱ %s", timeStr)

			// Git activity
			changed := countGitChanges("--name-only")
			untracked := countGitUntracked()
			if changed > 0 || untracked > 0 {
				line += fmt.Sprintf(" \033[90m│\033[0m 📝 %d changed, %d new", changed, untracked)
			}

			fmt.Fprint(os.Stderr, line)
		}
	}
}

func countGitChanges(flag string) int {
	out, err := exec.Command("git", "diff", flag).Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0
	}
	return len(lines)
}

func countGitUntracked() int {
	out, err := exec.Command("git", "ls-files", "--others", "--exclude-standard").Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0
	}
	return len(lines)
}

func notify(title, body string) {
	fmt.Print("\a")                                              // bell
	fmt.Fprintf(os.Stderr, "\033]9;%s\033\\", title+": "+body)  // OSC 9
	fmt.Fprintf(os.Stderr, "\033]99;i=ralph:d=0;%s\033\\", title+": "+body) // OSC 99
	fmt.Println()
	fmt.Printf("🔔 %s: %s\n", title, body)
	fmt.Println()
}

func printLoopStatus(state *model.LoopState) {
	data, _ := json.MarshalIndent(state, "", "  ")
	fmt.Printf("\n  Status: %s | Phase: %s | Step: %s",
		state.Status, state.Phase, state.Step)
	if state.Task != "" {
		fmt.Printf(" | Task: %s", state.Task)
	}
	fmt.Println()
	_ = data
}

func init() {
	ralphCmd.Flags().String("phase", "", "Phase to run")
	ralphCmd.Flags().Int("max", 10, "Max iterations per task")
	ralphCmd.Flags().String("task", "", "Lock to specific task")
	rootCmd.AddCommand(ralphCmd)
}
