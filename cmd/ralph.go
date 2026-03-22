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
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
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
						// Update task status in main task file
						task, err := taskStore.Read(taskID)
						if err == nil && task != nil {
							task.Status = "done"
							taskStore.Write(task)
						}
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
}

func spawnWorker(ctx context.Context, taskID string) *exec.Cmd {
	proc := exec.CommandContext(ctx, os.Args[0], "ralph", "worker", "run", "--task", taskID, "--root-dir", cfg.Root)
	proc.Stdout = nil
	proc.Stderr = nil

	if err := proc.Start(); err != nil {
		fmt.Printf("Error spawning worker for %s: %v\n", taskID, err)
		return nil
	}

	// Wait in background so ProcessState gets populated on exit
	go proc.Wait()

	return proc
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

func init() {
	// Flags for ralph run subcommand
	ralphRunCmd.Flags().String("phase", "", "Phase to run")
	ralphRunCmd.Flags().String("task", "", "Single task (legacy)")

	// Add subcommands to ralph
	ralphCmd.AddCommand(ralphRunCmd)
	ralphCmd.AddCommand(ralphStatusCmd)
	ralphCmd.AddCommand(workerCmd)

	rootCmd.AddCommand(ralphCmd)
}
