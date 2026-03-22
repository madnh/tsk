package main

import "github.com/madnh/tsk/cmd"

// Injected at build time via ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	cmd.SetVersion(Version, Commit, Date)
	cmd.Execute()
}
