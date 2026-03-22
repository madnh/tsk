package git

import (
	"os"
	"os/exec"
	"strings"
)

// Create adds a git worktree at the given path with the specified branch
// If the branch doesn't exist, it's created from HEAD
func Create(repoRoot, worktreePath, branchName string) error {
	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); err == nil {
		return nil // Already exists, skip creation
	}

	// Try to create worktree with new branch
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, worktreePath, "HEAD")
	cmd.Dir = repoRoot
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		// Maybe branch already exists, try without -b flag
		cmd = exec.Command("git", "worktree", "add", worktreePath, branchName)
		cmd.Dir = repoRoot
		return cmd.Run()
	}

	return nil
}

// Remove deletes a git worktree and its branch
func Remove(repoRoot, worktreePath string) error {
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoRoot
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return err
	}

	// Prune stale references
	cmd = exec.Command("git", "worktree", "prune")
	cmd.Dir = repoRoot
	return cmd.Run()
}

// RebaseOntoMain rebases the branch in the worktree onto the main branch
// Returns error if there are conflicts
func RebaseOntoMain(worktreePath, mainBranch string) error {
	// Fetch latest from origin
	cmd := exec.Command("git", "fetch", "origin", mainBranch)
	cmd.Dir = worktreePath
	if err := cmd.Run(); err != nil {
		// Ignore fetch errors, might be offline
	}

	// Rebase onto main
	cmd = exec.Command("git", "rebase", "origin/"+mainBranch)
	cmd.Dir = worktreePath
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		// Check for conflicts
		if HasConflicts(worktreePath) {
			// Abort the rebase
			exec.Command("git", "rebase", "--abort").Dir = worktreePath
			return err
		}
		return err
	}

	return nil
}

// MergeToMain merges the given branch into main using --ff-only
func MergeToMain(repoRoot, branchName string) error {
	cmd := exec.Command("git", "merge", "--ff-only", branchName)
	cmd.Dir = repoRoot
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// HasConflicts checks if the worktree has unresolved merge conflicts
func HasConflicts(worktreePath string) bool {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = worktreePath

	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	conflicts := strings.TrimSpace(string(out))
	return conflicts != ""
}

// HasRemote checks if the repository has a remote configured
func HasRemote(repoRoot string) bool {
	cmd := exec.Command("git", "remote")
	cmd.Dir = repoRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	remotes := strings.TrimSpace(string(out))
	return remotes != ""
}
