package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/config"
	"github.com/madnh/tsk/internal/engine"
	"github.com/madnh/tsk/internal/model"
	"github.com/madnh/tsk/internal/output"
	"github.com/madnh/tsk/internal/store"
)

var ralphCmd = &cobra.Command{
	Use:   "ralph",
	Short: "Autonomous task execution supervisor",
	Long:  "Manage parallel task execution with per-type workflows",
}

var ralphRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the supervisor for a phase",
	Run: func(cmd *cobra.Command, args []string) {
		phaseFlag, _ := cmd.Flags().GetString("phase")
		taskFlag, _ := cmd.Flags().GetString("task")

		// Support legacy --task flag by spawning single worker
		if taskFlag != "" {
			fmt.Println("Note: --task flag spawns a single worker for that task")
			cmd.Name()
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle signals
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		go func() {
			<-sigCh
			fmt.Fprintln(os.Stderr, "\nShutting down supervisor...")
			cancel()
		}()

		supervisorStore := store.NewSupervisorStore(cfg.LoopDir)

		// Init or resume supervisor
		if !supervisorStore.StateExists() {
			initSupervisor(phaseFlag, supervisorStore)
		} else if phaseFlag != "" {
			fmt.Println("Re-initializing supervisor with new phase...")
			initSupervisor(phaseFlag, supervisorStore)
		}

		runSupervisor(ctx, supervisorStore)
	},
}

func initSupervisor(phaseNum string, supervisorStore *store.SupervisorStore) {
	supervisorStore.EnsureDir()
	phases, _ := phaseStore.All()
	if len(phases) == 0 {
		output.Fail("No phases found.")
	}

	var currentPhase *model.Phase
	if phaseNum != "" {
		for _, p := range phases {
			if p.Num == phaseNum {
				currentPhase = p
				break
			}
		}
		if currentPhase == nil {
			output.Fail(fmt.Sprintf("Phase %s not found.", phaseNum))
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

	if !engine.IsPhaseRunnable(currentPhase) {
		output.Fail(fmt.Sprintf("Phase %s is '%s'. Only 'ready' or 'in_progress' phases can be run.",
			phaseNum, currentPhase.Status))
	}

	if currentPhase.Status == "ready" {
		currentPhase.Status = "in_progress"
		phaseStore.Write(currentPhase)
	}

	state := model.NewSupervisorState(currentPhase.Num)
	supervisorStore.WriteState(state)
	supervisorStore.Log(fmt.Sprintf("INIT phase=%s", currentPhase.Num))
	fmt.Printf("Started supervisor for phase %s\n", currentPhase.Num)
}

func runSupervisor(ctx context.Context, supervisorStore *store.SupervisorStore) {
	reconcileOrphanWorkers(supervisorStore)

	maxWorkers := config.GetMaxWorkers()
	pollInterval := time.Duration(config.GetSupervisorPoll()) * time.Second
	activeWorkers := make(map[string]*exec.Cmd)

	fmt.Printf("Supervisor running (max workers: ")
	if maxWorkers == 0 {
		fmt.Printf("unlimited")
	} else {
		fmt.Printf("%d", maxWorkers)
	}
	fmt.Printf(", poll: %v)\n", pollInterval)

	for {
		if ctx.Err() != nil {
			break
		}

		superState, err := supervisorStore.ReadState()
		if err != nil {
			fmt.Printf("Error reading supervisor state: %v\n", err)
			supervisorStore.Log(fmt.Sprintf("ERROR reading state: %v", err))
			return
		}

		if superState.Status != "running" {
			supervisorStore.Log(fmt.Sprintf("Phase complete, exiting (status=%s)", superState.Status))
			return
		}

		// Reap dead workers
		for taskID, proc := range activeWorkers {
			if proc.ProcessState != nil && proc.ProcessState.Exited() {
				workerStore := store.NewWorkerStore(cfg.TasksDir, taskID)
				wState, _ := workerStore.ReadState()
				if wState != nil {
					if wState.Status == "done" {
						fmt.Printf("  ✓ %s complete\n", taskID)
					} else {
						fmt.Printf("  ⚠ %s exited with status: %s\n", taskID, wState.Status)
					}
					supervisorStore.UpdateWorker(taskID, wState.Status)
				}
				delete(activeWorkers, taskID)
			}
		}

		// Spawn new workers for eligible tasks
		allTasks, _ := taskStore.All()
		for _, task := range allTasks {
			if task.Phase != superState.Phase {
				continue
			}
			if task.Status != "pending" && task.Status != "in_progress" {
				continue
			}
			// Check if worker already completed (state may be ahead of task file)
			workerStore := store.NewWorkerStore(cfg.TasksDir, task.ID)
			if workerStore.StateExists() {
				wState, _ := workerStore.ReadState()
				if wState != nil && wState.Status == "done" {
					task.Status = "done"
					taskStore.Write(task)
					supervisorStore.Log(fmt.Sprintf("SYNC task %s status to done (worker already completed)", task.ID))
					fmt.Printf("  ✓ %s already done (synced)\n", task.ID)
					continue
				}
			}
			if engine.IsBlocked(task, allTasks) {
				continue
			}
			if _, active := activeWorkers[task.ID]; active {
				continue
			}

			// Check max workers
			if maxWorkers > 0 && len(activeWorkers) >= maxWorkers {
				break
			}

			proc := spawnWorker(ctx, task.ID)
			if proc == nil {
				continue
			}
			activeWorkers[task.ID] = proc
			supervisorStore.AddWorker(task.ID, proc.Process.Pid)
			fmt.Printf("  ▶ Spawned worker for %s (PID %d)\n", task.ID, proc.Process.Pid)
		}

		// Check phase completion
		if len(activeWorkers) == 0 {
			allDone := true
			for _, task := range allTasks {
				if task.Phase == superState.Phase && task.Status != "done" {
					allDone = false
					break
				}
			}
			if allDone {
				superState.Status = "complete"
				supervisorStore.WriteState(superState)
				supervisorStore.Log("PHASE_COMPLETE")
				fmt.Printf("✓ Phase %s complete!\n", superState.Phase)
				return
			}
		}

		supervisorStore.Log(fmt.Sprintf("POLL active_workers=%d", len(activeWorkers)))
		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			break
		}
	}

	// Graceful shutdown: SIGTERM all workers, wait, then SIGKILL
	if len(activeWorkers) > 0 {
		fmt.Printf("Cleaning up %d worker(s)...\n", len(activeWorkers))
		for _, proc := range activeWorkers {
			if proc.Process != nil {
				proc.Process.Signal(syscall.SIGTERM)
			}
		}
		deadline := time.After(10 * time.Second)
		for len(activeWorkers) > 0 {
			select {
			case <-deadline:
				for taskID, proc := range activeWorkers {
					if proc.Process != nil {
						proc.Process.Kill()
					}
					fmt.Printf("  ✗ Force-killed worker for %s\n", taskID)
					delete(activeWorkers, taskID)
				}
				return
			case <-time.After(500 * time.Millisecond):
				for id, proc := range activeWorkers {
					if proc.ProcessState != nil && proc.ProcessState.Exited() {
						fmt.Printf("  ✓ Worker %s exited cleanly\n", id)
						delete(activeWorkers, id)
					}
				}
			}
		}
		fmt.Println("All workers stopped.")
	}
}

func spawnWorker(ctx context.Context, taskID string) *exec.Cmd {
	proc := exec.CommandContext(ctx, os.Args[0], "ralph", "worker", "run", "--task", taskID, "--root-dir", cfg.Root)
	proc.Stdout = nil
	proc.Stderr = nil
	proc.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
	}
	proc.Cancel = func() error {
		return proc.Process.Signal(syscall.SIGTERM)
	}
	proc.WaitDelay = 10 * time.Second

	if err := proc.Start(); err != nil {
		fmt.Printf("Error spawning worker for %s: %v\n", taskID, err)
		return nil
	}

	// Wait in background so ProcessState gets populated on exit
	go proc.Wait()

	return proc
}

// reconcileOrphanWorkers kills any workers left running from a previous supervisor session.
func reconcileOrphanWorkers(supervisorStore *store.SupervisorStore) {
	state, err := supervisorStore.ReadState()
	if err != nil {
		return
	}
	orphansFound := false
	for _, w := range state.Workers {
		if w.Status != "running" || w.PID <= 0 {
			continue
		}
		proc, err := os.FindProcess(w.PID)
		if err != nil {
			continue
		}
		// Signal 0 checks if process exists without killing it
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Process no longer exists — mark as failed
			supervisorStore.UpdateWorker(w.TaskID, "failed")
			supervisorStore.Log(fmt.Sprintf("RECONCILE worker %s (PID %d) no longer running, marked failed", w.TaskID, w.PID))
			continue
		}
		// Process still alive — it's an orphan
		orphansFound = true
		fmt.Printf("  ⚠ Killing orphan worker PID %d (task %s)\n", w.PID, w.TaskID)
		proc.Signal(syscall.SIGTERM)
		supervisorStore.UpdateWorker(w.TaskID, "failed")
		supervisorStore.Log(fmt.Sprintf("RECONCILE killed orphan worker %s (PID %d)", w.TaskID, w.PID))
	}
	if orphansFound {
		// Give orphans a moment to clean up
		time.Sleep(2 * time.Second)
	}
}

var ralphStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show supervisor status",
	Run: func(cmd *cobra.Command, args []string) {
		supervisorStore := store.NewSupervisorStore(cfg.LoopDir)
		if !supervisorStore.StateExists() {
			fmt.Println("Supervisor not running")
			return
		}

		superState, err := supervisorStore.ReadState()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		fmt.Printf("\nSupervisor Status:\n")
		fmt.Printf("  Phase: %s\n", superState.Phase)
		fmt.Printf("  Status: %s\n", superState.Status)
		fmt.Printf("  Active Workers: %d\n", len(superState.Workers))
		fmt.Println()

		if len(superState.Workers) == 0 {
			fmt.Println("  (no workers)")
			return
		}

		fmt.Println("  Workers:")
		for _, w := range superState.Workers {
			fmt.Printf("    - %s: %s (PID %d)\n", w.TaskID, w.Status, w.PID)
		}
	},
}

var ralphCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Kill orphan workers and clean up stale state",
	Run: func(cmd *cobra.Command, args []string) {
		supervisorStore := store.NewSupervisorStore(cfg.LoopDir)
		if !supervisorStore.StateExists() {
			fmt.Println("No supervisor state found.")
			return
		}

		state, err := supervisorStore.ReadState()
		if err != nil {
			fmt.Printf("Error reading supervisor state: %v\n", err)
			return
		}

		found := 0
		for _, w := range state.Workers {
			if w.PID <= 0 {
				continue
			}
			proc, err := os.FindProcess(w.PID)
			if err != nil {
				continue
			}
			alive := proc.Signal(syscall.Signal(0)) == nil
			if w.Status == "running" {
				found++
				if alive {
					fmt.Printf("  ⚠ Killing orphan worker PID %d (task %s)\n", w.PID, w.TaskID)
					proc.Signal(syscall.SIGTERM)
				} else {
					fmt.Printf("  ✗ Stale worker entry PID %d (task %s) — process already dead\n", w.PID, w.TaskID)
				}
				supervisorStore.UpdateWorker(w.TaskID, "failed")
			} else if alive {
				found++
				fmt.Printf("  ⚠ Killing lingering worker PID %d (task %s, status %s)\n", w.PID, w.TaskID, w.Status)
				proc.Signal(syscall.SIGTERM)
			}
		}

		if found == 0 {
			fmt.Println("No orphan or stale workers found.")
		} else {
			fmt.Printf("Cleaned up %d worker(s).\n", found)
			supervisorStore.Log(fmt.Sprintf("CLEANUP killed/cleaned %d workers", found))
		}
	},
}

func init() {
	// Flags for ralph run subcommand
	ralphRunCmd.Flags().String("phase", "", "Phase to run")
	ralphRunCmd.Flags().String("task", "", "Single task (legacy)")

	// Add subcommands to ralph
	ralphCmd.AddCommand(ralphRunCmd)
	ralphCmd.AddCommand(ralphStatusCmd)
	ralphCmd.AddCommand(ralphCleanupCmd)
	ralphCmd.AddCommand(workerCmd)

	rootCmd.AddCommand(ralphCmd)
}
