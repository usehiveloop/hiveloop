package drift

import (
	"os/exec"
	"path/filepath"
	"strings"
)

type Status int

const (
	NoChange Status = iota // HEAD matches stored commit
	Changed                // HEAD differs, diff available
	NoDrift                // not a git repo or can't detect
	Error                  // git command failed
)

type Result struct {
	Status       Status
	CurrentHead  string
	StoredHead   string
	ChangedFiles []string // relative to repo root, only supported extensions
	Warning      string
}

// CurrentHead returns the current git HEAD for a repo, or "unknown".
func CurrentHead(repoPath string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// Detect compares current HEAD against storedCommit and returns changed files.
func Detect(repoPath, storedCommit string, supportedExts []string) Result {
	currentHead := CurrentHead(repoPath)
	if currentHead == "unknown" {
		return Result{Status: NoDrift, Warning: "not a git repository"}
	}

	if currentHead == storedCommit {
		return Result{Status: NoChange, CurrentHead: currentHead, StoredHead: storedCommit}
	}

	// Get changed files
	diffArgs := []string{"diff", "--name-only", storedCommit, currentHead}
	// Also include uncommitted changes
	cmd := exec.Command("git", diffArgs...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		// Stored commit might not be reachable (shallow clone, force push)
		return Result{
			Status:  NoDrift,
			Warning: "stored commit " + storedCommit[:minLen(len(storedCommit), 8)] + " not reachable, possibly shallow clone",
		}
	}

	// Also get uncommitted changes
	uncommittedCmd := exec.Command("git", "diff", "--name-only")
	uncommittedCmd.Dir = repoPath
	uncommittedOut, _ := uncommittedCmd.Output()

	// Merge both sets
	allFiles := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			allFiles[line] = true
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(string(uncommittedOut)), "\n") {
		if line != "" {
			allFiles[line] = true
		}
	}

	// Filter to supported extensions
	extSet := make(map[string]bool, len(supportedExts))
	for _, ext := range supportedExts {
		extSet[ext] = true
	}

	var changedFiles []string
	for filePath := range allFiles {
		ext := filepath.Ext(filePath)
		if extSet[ext] {
			changedFiles = append(changedFiles, filePath)
		}
	}

	if len(changedFiles) == 0 {
		// Files changed but none are source files we care about
		return Result{Status: NoChange, CurrentHead: currentHead, StoredHead: storedCommit}
	}

	return Result{
		Status:       Changed,
		CurrentHead:  currentHead,
		StoredHead:   storedCommit,
		ChangedFiles: changedFiles,
	}
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
