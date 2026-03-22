package config

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the loaded configuration
type Config struct {
	Root      string
	TasksDir  string
	ItemsDir  string
	PhasesDir string
	LoopDir   string
}

// ResolveRoot finds the project root directory using priority order:
// 1. --root-dir flag (passed as parameter)
// 2. TSK_ROOT_DIR env var
// 3. Walk up from cwd looking for tsk.yml
// 4. git rev-parse --show-toplevel
// 5. Current directory (fallback)
func ResolveRoot(flagRoot string) string {
	// 1. Flag
	if flagRoot != "" {
		return flagRoot
	}

	// 2. Env var
	if env := os.Getenv("TSK_ROOT_DIR"); env != "" {
		return env
	}

	// 3. Walk up from cwd looking for tsk.yml
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for {
			if _, err := os.Stat(filepath.Join(dir, "tsk.yml")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// 4. git
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		root := string(out)
		// Trim trailing newline
		if len(root) > 0 && root[len(root)-1] == '\n' {
			root = root[:len(root)-1]
		}
		return root
	}

	// 5. Fallback to cwd
	if cwd != "" {
		return cwd
	}
	return "."
}

// Load reads tsk.yml and returns Config
func Load(flagRoot string) *Config {
	root := ResolveRoot(flagRoot)

	viper.SetConfigName("tsk")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(root)
	viper.ReadInConfig() // ignore error — config is optional

	cfg := &Config{
		Root:      root,
		TasksDir:  filepath.Join(root, "tasks"),
		ItemsDir:  filepath.Join(root, "tasks", "items"),
		PhasesDir: filepath.Join(root, "tasks", "phases"),
		LoopDir:   filepath.Join(root, "tasks", "loop"),
	}

	return cfg
}

// GetMaxIterations returns max iterations from config, default 10
func GetMaxIterations() int {
	v := viper.GetInt("ralph.max_iterations")
	if v == 0 {
		return 10
	}
	return v
}

// GetCooldown returns cooldown seconds between steps, default 60
func GetCooldown() int {
	v := viper.GetInt("ralph.cooldown")
	if v == 0 {
		return 60
	}
	return v
}

// GetRetryMax returns max retries on rate limit, default 10
func GetRetryMax() int {
	v := viper.GetInt("ralph.retry_max")
	if v == 0 {
		return 10
	}
	return v
}

// GetRetryWait returns retry wait seconds, default 600
func GetRetryWait() int {
	v := viper.GetInt("ralph.retry_wait")
	if v == 0 {
		return 600
	}
	return v
}

// GetClaudeCommand returns the claude command, default "claude"
func GetClaudeCommand() string {
	cmd := viper.GetString("ralph.claude.command")
	if cmd == "" {
		return "claude"
	}
	return cmd
}

// GetClaudeArgs returns the claude args, default ["-p", "--dangerously-skip-permissions"]
func GetClaudeArgs() []string {
	args := viper.GetStringSlice("ralph.claude.args")
	if len(args) == 0 {
		return []string{"-p", "--dangerously-skip-permissions"}
	}
	return args
}

// GetAutoPush returns whether to auto-push after each loop step
func GetAutoPush() bool {
	return viper.GetBool("ralph.auto_push")
}

// GetDefaultPriority returns default task priority from config
func GetDefaultPriority() string {
	p := viper.GetString("task.default_priority")
	if p == "" {
		return "medium"
	}
	return p
}

// GetDefaultType returns default task type from config
func GetDefaultType() string {
	t := viper.GetString("task.default_type")
	if t == "" {
		return "feature"
	}
	return t
}

// PhaseConfig represents a phase definition from tsk.yml
type PhaseConfig struct {
	Num         string `mapstructure:"num"`
	Name        string `mapstructure:"name"`
	Description string `mapstructure:"description"`
}

// GetPhases returns phase definitions from config
func GetPhases() []PhaseConfig {
	var phases []PhaseConfig
	viper.UnmarshalKey("phases", &phases)
	return phases
}

// GetUpdateCheckOnStartup returns whether to check for updates at startup
func GetUpdateCheckOnStartup() bool {
	return viper.GetBool("update.check_on_startup")
}

// GetUpdateTimeout returns the timeout in seconds for startup update checks, default 5
func GetUpdateTimeout() int {
	v := viper.GetInt("update.timeout_seconds")
	if v == 0 {
		return 5
	}
	return v
}
