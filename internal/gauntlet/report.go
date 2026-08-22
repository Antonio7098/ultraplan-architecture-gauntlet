package gauntlet

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func StatusText(r *Review) string {
	counts := map[string]map[TaskStatus]int{}
	for _, s := range stages {
		counts[s] = map[TaskStatus]int{}
	}
	for _, t := range r.Tasks {
		counts[t.Stage][t.Status]++
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Target: %s @ %s (dirty=%t)\nWorkspace: %s @ %s (dirty=%t)\nModel: %s\nTasks: %d\n\n", r.TargetPath, r.TargetCommit, r.TargetDirty, r.WorkspacePath, r.WorkspaceCommit, r.WorkspaceDirty, r.Model, len(r.Tasks))
	for _, s := range stages {
		c := counts[s]
		fmt.Fprintf(&b, "%-10s total=%-3d pending=%-3d running=%-3d succeeded=%-3d failed=%-3d\n", s, c[StatusPending]+c[StatusRunning]+c[StatusSucceeded]+c[StatusFailed], c[StatusPending], c[StatusRunning], c[StatusSucceeded], c[StatusFailed])
	}
	return b.String()
}

func NextStage(r *Review) string {
	for _, s := range stages {
		for _, t := range r.Tasks {
			if t.Stage == s && t.Status != StatusSucceeded {
				return s
			}
		}
	}
	return "complete"
}

func BuildIndex(out string, r *Review) (string, error) {
	var b strings.Builder
	b.WriteString("# Architecture Gauntlet Run Index\n\n")
	fmt.Fprintf(&b, "Target: `%s`\n\n", r.TargetPath)
	for _, s := range stages {
		fmt.Fprintf(&b, "## %s\n\n", strings.Title(s))
		var tasks []Task
		for _, t := range r.Tasks {
			if t.Stage == s {
				tasks = append(tasks, t)
			}
		}
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
		for _, t := range tasks {
			result := filepath.Join("runs", t.ID, "stdout.md")
			fmt.Fprintf(&b, "- `%s` — **%s** — %s — [%s](%s)\n", t.ID, t.Status, t.Title, "result", result)
		}
		b.WriteString("\n")
	}
	path := filepath.Join(out, "INDEX.md")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
