package cmd

import (
	"context"
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
	"github.com/madnh/tsk/internal/git"
	"github.com/madnh/tsk/internal/model"
	"github.com/madnh/tsk/internal/output"
	"github.com/madnh/tsk/internal/prompt"
	"github.com/madnh/tsk/internal/store"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Manage individual task workers",
}

var workerRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a worker for a specific task",
	Run: func(cmd *cobra.Command, args []string) {
		taskID, _ := cmd.Flags().GetString("task")
		if taskID == "" {
			output.Fail("--task flag is required")
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle signals
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Fprintln(os.Stderr, "\nShutting down worker...")
			cancel()
		}()

		runWorker(ctx, taskID, cfg)
	},
}

var workerResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume a blocked worker",
	Run: func(cmd *cobra.Command, args []string) {
		taskID, _ := cmd.Flags().GetString("task")
		if taskID == "" {
			output.Fail("--task flag is required")
		}

		workerStore := store.NewWorkerStore(cfg.TasksDir, taskID)
		state, err := workerStore.ReadState()
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to read worker state: %v", err))
		}

		if state.Status != "blocked" {
			output.Fail(fmt.Sprintf("Worker is not blocked (status: %s)", state.Status))
		}

		if !workerStore.FileExists("human-input.md") {
			output.Fail("No human-input.md found. Write guidance there first.")
		}

		state.Status = "running"
		workerStore.WriteState(state)
		fmt.Printf("Resuming worker for %s...\n", taskID)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Fprintln(os.Stderr, "\nShutting down worker...")
			cancel()
		}()

		runWorker(ctx, taskID, cfg)
	},
}

var workerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all workers",
	Run: func(cmd *cobra.Command, args []string) {
		workersDir := cfg.TasksDir + "/workers"
		entries, err := os.ReadDir(workersDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No workers yet.")
				return
			}
			output.Fail(fmt.Sprintf("Failed to read workers: %v", err))
		}

		if len(entries) == 0 {
			fmt.Println("No workers.")
			return
		}

		fmt.Println("Workers:")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("%-15s %-10s %-10s %-20s %-10s\n", "Task", "Step", "Status", "Started", "PID")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			taskID := entry.Name()
			workerStore := store.NewWorkerStore(cfg.TasksDir, taskID)
			state, err := workerStore.ReadState()
			if err != nil {
				continue
			}

			step := state.CurrentStep()
			if step == "" {
				step = "-"
			}

			started := state.StartedAt
			if started != "" {
				// Parse and format
				if t, err := time.Parse(time.RFC3339, started); err == nil {
					started = t.Format("2006-01-02 15:04")
				}
			}

			fmt.Printf("%-15s %-10s %-10s %-20s %-10d\n", taskID, step, state.Status, started, state.PID)
		}
	},
}

var workerKillCmd = &cobra.Command{
	Use:   "kill",
	Short: "Kill a worker by task ID",
	Run: func(cmd *cobra.Command, args []string) {
		taskID, _ := cmd.Flags().GetString("task")
		if taskID == "" {
			output.Fail("--task flag is required")
		}

		workerStore := store.NewWorkerStore(cfg.TasksDir, taskID)
		state, err := workerStore.ReadState()
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to read worker state: %v", err))
		}

		if state.PID == 0 {
			output.Fail("Worker PID not found")
		}

		proc, err := os.FindProcess(state.PID)
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to find process: %v", err))
		}

		if err := proc.Signal(syscall.SIGTERM); err != nil {
			output.Fail(fmt.Sprintf("Failed to kill process: %v", err))
		}

		fmt.Printf("Killed worker for %s (PID %d)\n", taskID, state.PID)
	},
}

var workerLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show worker logs",
	Run: func(cmd *cobra.Command, args []string) {
		taskID, _ := cmd.Flags().GetString("task")
		if taskID == "" {
			output.Fail("--task flag is required")
		}

		workerStore := store.NewWorkerStore(cfg.TasksDir, taskID)
		entries, err := workerStore.ReadLogEntries()
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to read logs: %v", err))
		}

		if len(entries) == 0 {
			fmt.Println("No logs.")
			return
		}

		for _, entry := range entries {
			if entry != "" {
				fmt.Println(entry)
			}
		}
	},
}

func runWorker(ctx context.Context, taskID string, cfg *config.Config) {
	taskStore := store.NewTaskStore(cfg.ItemsDir)
	workerStore := store.NewWorkerStore(cfg.TasksDir, taskID)
	phaseStore := store.NewPhaseStore(cfg.PhasesDir)

	// Read task
	task, err := taskStore.Read(taskID)
	if err != nil || task == nil {
		output.Fail(fmt.Sprintf("Failed to read task: %v", err))
	}

	// Load or init worker state
	var state *model.WorkerState
	if workerStore.StateExists() {
		state, err = workerStore.ReadState()
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to read worker state: %v", err))
		}
	} else {
		workerStore.EnsureDir()
		workflow := config.GetWorkflow(task.Type)
		state = model.NewWorkerState(taskID, task.Type, workflow, config.GetMaxIterations())
		state.WorktreePath = fmt.Sprintf("worktrees/%s", taskID)
		state.BranchName = fmt.Sprintf("task/%s", taskID)
	}

	// Write PID
	state.PID = os.Getpid()
	workerStore.WriteState(state)
	workerStore.Log(fmt.Sprintf("START task=%s workflow=%v", taskID, state.Workflow))

	// Ensure worktree exists
	if err := git.Create(cfg.Root, state.WorktreePath, state.BranchName); err != nil {
		workerStore.Log(fmt.Sprintf("FAILED: git.Create: %v", err))
		output.Fail(fmt.Sprintf("Failed to create worktree: %v", err))
	}

	// Mark task in_progress if pending
	if task.Status == "pending" {
		task.Status = "in_progress"
		task.Started = time.Now().Format("2006-01-02")
		taskStore.Write(task)
	}

	cooldown := time.Duration(config.GetCooldown()) * time.Second
	maxRetries := config.GetRetryMax()
	retryCount := 0
	retryWait := time.Duration(config.GetRetryWait()) * time.Second

	// Main worker loop
	for {
		if ctx.Err() != nil {
			break
		}

		if state.Status != "running" {
			break
		}

		step := state.CurrentStep()
		if step == "" {
			workerStore.Log("COMPLETE")
			state.Status = "done"
			break
		}

		// Generate prompt
		gen := &prompt.Generator{
			LoopStore:  nil,
			TaskStore:  taskStore,
			PhaseStore: phaseStore,
			RootDir:    cfg.Root,
		}

		promptText, err := gen.GenerateWorker(state, task, workerStore)
		if err != nil {
			workerStore.Log(fmt.Sprintf("FAILED: prompt generation: %v", err))
			state.Status = "failed"
			break
		}

		// Record step start
		state.StepStartedAt = time.Now().Format(time.RFC3339)
		workerStore.WriteState(state)
		workerStore.Log(fmt.Sprintf("START step=%s iteration=%d", step, state.Iteration+1))

		// Start progress monitor
		var monitorStop context.CancelFunc
		var monitorCtx context.Context
		monitorCtx, monitorStop = context.WithCancel(ctx)
		var monitorWg sync.WaitGroup
		monitorWg.Add(1)
		go func() {
			defer monitorWg.Done()
			runWorkerMonitor(monitorCtx, step, taskID)
		}()

		// Execute claude in worktree
		claudeOutput, exitCode := executeClaudeCmdInDir(ctx, promptText, state.WorktreePath)

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
			fmt.Printf("  Claude exited with code %d (step=%s, task=%s)\n", exitCode, step, taskID)
			fmt.Printf("  Attempt %d/%d for this step\n", retryCount, maxRetries)
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			if retryCount >= maxRetries {
				fmt.Println()
				fmt.Println("Max retries reached. Worker pausing.")
				state.Status = "blocked"
				state.BlockedReason = "Rate limit retries exhausted"
				break
			}

			fmt.Println()
			fmt.Printf("Waiting %v for rate limit reset...\n", retryWait)
			fmt.Println("  (Press Ctrl+C to stop)")
			select {
			case <-time.After(retryWait):
			case <-ctx.Done():
				workerStore.WriteState(state)
				return
			}
			continue
		}

		retryCount = 0

		// Advance state machine
		eng := &engine.WorkerEngine{
			WorkerStore: workerStore,
			TaskStore:   taskStore,
		}
		result, err := eng.Advance(state)
		if err != nil {
			workerStore.Log(fmt.Sprintf("FAILED: advance: %v", err))
			state.Status = "failed"
			break
		}

		if state.Status == "done" {
			// Merge to main
			if err := handleWorkerCompletion(cfg, state, taskID, workerStore); err != nil {
				workerStore.Log(fmt.Sprintf("FAILED: merge: %v", err))
				state.Status = "failed"
			}
			break
		}

		if state.Status != "running" {
			break
		}

		// Auto push if configured
		if config.GetAutoPush() {
			pushOut, pushErr := exec.Command("git", "push").CombinedOutput()
			if pushErr != nil {
				workerStore.Log(fmt.Sprintf("PUSH_FAILED: %s", strings.TrimSpace(string(pushOut))))
			} else {
				workerStore.Log("PUSH_OK")
			}
		}

		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("  ✓ Step done — %s\n", result.Action)
		fmt.Printf("  Cooldown %v. Press Ctrl+C to stop safely.\n", cooldown)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		select {
		case <-time.After(cooldown):
		case <-ctx.Done():
			workerStore.WriteState(state)
			return
		}
	}

	workerStore.WriteState(state)
}

func handleWorkerCompletion(cfg *config.Config, state *model.WorkerState, taskID string, workerStore *store.WorkerStore) error {
	workerStore.Log("MERGING to main...")

	// Rebase onto main
	if err := git.RebaseOntoMain(state.WorktreePath, "main"); err != nil {
		if git.HasConflicts(state.WorktreePath) {
			state.Status = "blocked"
			state.BlockedReason = "Merge conflict — resolve manually and resume"
			return err
		}
		return err
	}

	// Merge to main
	if err := git.MergeToMain(cfg.Root, state.BranchName); err != nil {
		return err
	}

	workerStore.Log("MERGED to main")

	// Remove worktree
	if err := git.Remove(cfg.Root, state.WorktreePath); err != nil {
		workerStore.Log(fmt.Sprintf("WARN: failed to remove worktree: %v", err))
	}

	workerStore.Log("COMPLETE")
	return nil
}

func executeClaudeCmdInDir(ctx context.Context, promptText, dir string) (string, int) {
	claudeCmd := config.GetClaudeCommand()
	claudeArgs := config.GetClaudeArgs()

	cmd := exec.CommandContext(ctx, claudeCmd, claudeArgs...)
	cmd.Dir = dir
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

func runWorkerMonitor(ctx context.Context, step, taskID string) {
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

			line := fmt.Sprintf("\r\033[K  %s \033[1m%s\033[0m %s", char, step, taskID)
			line += fmt.Sprintf(" \033[90m│\033[0m ⏱ %s", timeStr)

			fmt.Fprint(os.Stderr, line)
		}
	}
}

func init() {
	workerCmd.AddCommand(workerRunCmd)
	workerCmd.AddCommand(workerResumeCmd)
	workerCmd.AddCommand(workerStatusCmd)
	workerCmd.AddCommand(workerKillCmd)
	workerCmd.AddCommand(workerLogsCmd)

	workerRunCmd.Flags().String("task", "", "Task ID to run")
	workerResumeCmd.Flags().String("task", "", "Task ID to resume")
	workerKillCmd.Flags().String("task", "", "Task ID to kill")
	workerLogsCmd.Flags().String("task", "", "Task ID to view logs for")

	rootCmd.AddCommand(workerCmd)
}
