package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/madnh/tsk/internal/engine"
	"github.com/madnh/tsk/internal/model"
	"github.com/madnh/tsk/internal/output"
	"github.com/madnh/tsk/internal/prompt"
)

var loopCmd = &cobra.Command{
	Use:   "loop",
	Short: "Autonomous execution loop commands",
}

var loopInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the autonomous execution loop",
	Run: func(cmd *cobra.Command, args []string) {
		loopStore.EnsureDir()

		if loopStore.StateExists() {
			output.Fail("Loop already initialized. Use loop reset first.")
		}

		phases, _ := phaseStore.All()
		if len(phases) == 0 {
			output.Fail("No phases found.")
		}

		phaseFlag, _ := cmd.Flags().GetString("phase")
		maxFlag, _ := cmd.Flags().GetInt("max")
		taskFlag, _ := cmd.Flags().GetString("task")

		if maxFlag == 0 {
			maxFlag = 10
		}

		var currentPhase *model.Phase
		if phaseFlag != "" {
			for _, p := range phases {
				if p.Num == phaseFlag {
					currentPhase = p
					break
				}
			}
			if currentPhase == nil {
				output.Fail(fmt.Sprintf("Phase %s not found.", phaseFlag))
			}
			if !engine.IsPhaseRunnable(currentPhase) {
				output.Fail(fmt.Sprintf("Phase %s is '%s'. Only 'ready' or 'in_progress' phases can be run. Change status first: tsk phase %s --status ready",
					phaseFlag, currentPhase.Status, phaseFlag))
			}
		} else {
			for _, p := range phases {
				if engine.IsPhaseRunnable(p) {
					currentPhase = p
					break
				}
			}
			if currentPhase == nil {
				output.Fail("No runnable phases (status must be 'ready' or 'in_progress'). Set a phase to 'ready' first.")
			}
		}

		// Auto-transition ready → in_progress
		if currentPhase.Status == "ready" {
			currentPhase.Status = "in_progress"
			phaseStore.Write(currentPhase)
		}

		lockTask := taskFlag != ""
		state := &model.LoopState{
			Phase:           currentPhase.Num,
			Task:            taskFlag,
			Step:            model.StepAnalyze,
			Iteration:       0,
			MaxIterations:   maxFlag,
			TotalIterations: 0,
			Status:          model.LoopRunning,
			StartedAt:       todayStr(),
			LockTask:        lockTask,
		}

		loopStore.WriteState(state)
		logEntry := fmt.Sprintf("INIT phase=%s max=%d", currentPhase.Num, maxFlag)
		if lockTask {
			logEntry += fmt.Sprintf(" lock=%s", taskFlag)
		}
		loopStore.Log(logEntry)

		output.Print(output.Result{
			Data: state,
			Pretty: func() {
				fmt.Printf("\n  %sRalph Loop initialized%s\n", output.Bold, output.Reset)
				fmt.Printf("  Phase: %s\n", state.Phase)
				fmt.Printf("  Step: %s\n", state.Step)
				fmt.Printf("  Max iterations per task: %d\n", state.MaxIterations)
				if lockTask {
					fmt.Printf("  Locked to task: %s\n", taskFlag)
				}
				fmt.Println()
			},
		})
	},
}

var loopStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show loop state and progress",
	Run: func(cmd *cobra.Command, args []string) {
		if !loopStore.StateExists() {
			output.Fail("Loop not initialized. Run loop init first.")
		}

		state, err := loopStore.ReadState()
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to read state: %v", err))
		}

		phases, _ := phaseStore.All()
		tasks, _ := taskStore.All()
		phasesDone := countTasks2(phases, func(p *model.Phase) bool { return p.Status == "done" })
		tasksDone := countTasks(tasks, func(t *model.Task) bool { return t.Status == "done" })

		output.Print(output.Result{
			Data: map[string]interface{}{
				"phase":           state.Phase,
				"task":            state.Task,
				"step":            state.Step,
				"iteration":       state.Iteration,
				"maxIterations":   state.MaxIterations,
				"totalIterations": state.TotalIterations,
				"status":          state.Status,
				"startedAt":       state.StartedAt,
				"lockTask":        state.LockTask,
				"progress": map[string]interface{}{
					"phases": map[string]int{"done": phasesDone, "total": len(phases)},
					"tasks":  map[string]int{"done": tasksDone, "total": len(tasks)},
				},
			},
			Pretty: func() {
				fmt.Printf("\n  %sRalph Loop Status%s\n", output.Bold, output.Reset)
				var statusColor string
				switch state.Status {
				case "running":
					statusColor = output.Green
				case "paused":
					statusColor = output.Yellow
				default:
					statusColor = output.Blue
				}
				fmt.Printf("  Status: %s%s%s\n", statusColor, state.Status, output.Reset)
				fmt.Printf("  Phase: %s | Step: %s\n", state.Phase, state.Step)
				if state.Task != "" {
					fmt.Printf("  Task: %s\n", state.Task)
				}
				fmt.Printf("  Iteration: %d/%d (total: %d)\n", state.Iteration, state.MaxIterations, state.TotalIterations)
				fmt.Printf("  Phases: %d/%d done\n", phasesDone, len(phases))
				fmt.Printf("  Tasks: %d/%d done\n", tasksDone, len(tasks))
				fmt.Println()
			},
		})
	},
}

var loopResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Clear loop state (preserves history)",
	Run: func(cmd *cobra.Command, args []string) {
		loopStore.Reset()
		loopStore.Log("RESET")

		output.Print(output.Result{
			Data: map[string]interface{}{"reset": true},
			Pretty: func() {
				fmt.Printf("\n  %sLoop state cleared%s\n\n", output.Bold, output.Reset)
			},
		})
	},
}

var loopPromptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Generate prompt for current step",
	Run: func(cmd *cobra.Command, args []string) {
		if !loopStore.StateExists() {
			output.Fail("Loop not initialized. Run loop init first.")
		}

		state, err := loopStore.ReadState()
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to read state: %v", err))
		}
		if state.Status != model.LoopRunning {
			output.Fail(fmt.Sprintf("Loop is %s. Cannot generate prompt.", state.Status))
		}

		gen := &prompt.Generator{
			LoopStore:  loopStore,
			TaskStore:  taskStore,
			PhaseStore: phaseStore,
			RootDir:    cfg.Root,
		}

		promptText, err := gen.Generate(state)
		if err != nil {
			output.Fail(err.Error())
		}

		// Record step start time
		state.StepStartedAt = time.Now().Format(time.RFC3339)
		loopStore.WriteState(state)

		logEntry := fmt.Sprintf("START step=%s phase=%s", state.Step, state.Phase)
		if state.Task != "" {
			logEntry += fmt.Sprintf(" task=%s", state.Task)
		}
		if state.Step == model.StepImplement {
			logEntry += fmt.Sprintf(" iter=%d/%d", state.Iteration+1, state.MaxIterations)
		}
		loopStore.Log(logEntry)

		// Output raw prompt to stdout (not through output system)
		fmt.Println(promptText)
	},
}

var loopAdvanceCmd = &cobra.Command{
	Use:   "advance",
	Short: "Advance the state machine",
	Run: func(cmd *cobra.Command, args []string) {
		if !loopStore.StateExists() {
			output.Fail("Loop not initialized. Run loop init first.")
		}

		state, err := loopStore.ReadState()
		if err != nil {
			output.Fail(fmt.Sprintf("Failed to read state: %v", err))
		}

		resume, _ := cmd.Flags().GetBool("resume")

		eng := &engine.LoopEngine{
			LoopStore:  loopStore,
			TaskStore:  taskStore,
			PhaseStore: phaseStore,
		}

		result, err := eng.Advance(state, resume)
		if err != nil {
			output.Fail(err.Error())
		}

		output.Print(output.Result{
			Data: result,
			Pretty: func() {
				var statusColor string
				switch result.Status {
				case "running":
					statusColor = output.Green
				case "paused":
					statusColor = output.Yellow
				default:
					statusColor = output.Blue
				}
				fmt.Printf("\n  %s%s%s — %s\n", statusColor, result.Status, output.Reset, result.Action)
				if result.Reason != "" {
					fmt.Printf("  Reason: %s\n", result.Reason)
				}
				fmt.Println()
			},
		})
	},
}

var loopLogCmd = &cobra.Command{
	Use:   "log",
	Short: "View loop history log",
	Run: func(cmd *cobra.Command, args []string) {
		entries, err := loopStore.ReadLogEntries()
		if err != nil {
			output.Fail("No loop history yet.")
		}
		if len(entries) == 0 {
			output.Fail("No loop history yet.")
		}

		tail, _ := cmd.Flags().GetInt("tail")
		display := entries
		if tail > 0 && tail < len(entries) {
			display = entries[len(entries)-tail:]
		}

		output.Print(output.Result{
			Data: map[string]interface{}{
				"entries": display,
				"total":   len(entries),
			},
			Pretty: func() {
				tailStr := ""
				if tail > 0 {
					tailStr = fmt.Sprintf(", showing last %d", tail)
				}
				fmt.Printf("\n  %sRalph Loop History%s (%d entries%s)\n\n",
					output.Bold, output.Reset, len(entries), tailStr)
				for _, line := range display {
					colored := line
					switch {
					case strings.Contains(line, "INIT"):
						colored = output.Cyan + line + output.Reset
					case strings.Contains(line, "RESET"):
						colored = output.Magenta + line + output.Reset
					case strings.Contains(line, "START"):
						colored = output.Yellow + line + output.Reset
					case strings.Contains(line, "SHIP_REJECTED"):
						colored = output.Red + line + output.Reset
					case strings.Contains(line, "SHIP"):
						colored = output.Green + line + output.Reset
					case strings.Contains(line, "REVISE"):
						colored = output.Yellow + line + output.Reset
					case strings.Contains(line, "BLOCKED"):
						colored = output.Red + line + output.Reset
					case strings.Contains(line, "PHASE_COMPLETE"):
						colored = output.Green + line + output.Reset
					case strings.Contains(line, "RESUME"):
						colored = output.Blue + line + output.Reset
					}
					fmt.Printf("  %s\n", colored)
				}
				fmt.Println()
			},
		})
	},
}

func countTasks2(phases []*model.Phase, fn func(*model.Phase) bool) int {
	n := 0
	for _, p := range phases {
		if fn(p) {
			n++
		}
	}
	return n
}

func init() {
	loopInitCmd.Flags().String("phase", "", "Phase number to run")
	loopInitCmd.Flags().Int("max", 10, "Max iterations per task")
	loopInitCmd.Flags().String("task", "", "Lock to specific task")

	loopAdvanceCmd.Flags().Bool("resume", false, "Resume from paused state")

	loopLogCmd.Flags().Int("tail", 0, "Show last N entries")

	loopCmd.AddCommand(loopInitCmd)
	loopCmd.AddCommand(loopStatusCmd)
	loopCmd.AddCommand(loopResetCmd)
	loopCmd.AddCommand(loopPromptCmd)
	loopCmd.AddCommand(loopAdvanceCmd)
	loopCmd.AddCommand(loopLogCmd)
	rootCmd.AddCommand(loopCmd)
}
