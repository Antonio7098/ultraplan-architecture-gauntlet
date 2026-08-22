package gauntlet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const stateFile = "review.json"

func Init(out, target, workspace, model string) (*Review, error) {
	if target == "" {
		return nil, errors.New("target path is required")
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(absTarget); err != nil {
		return nil, fmt.Errorf("target: %w", err)
	}
	absWorkspace := ""
	if workspace != "" {
		absWorkspace, err = filepath.Abs(workspace)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(absWorkspace); err != nil {
			return nil, fmt.Errorf("workspace: %w", err)
		}
	}
	if model == "" {
		model = "openrouter/stealth/ox-alpha"
	}
	absOut, err := filepath.Abs(out)
	if err != nil {
		return nil, err
	}
	targetCommit, targetDirty := gitSnapshot(absTarget)
	workspaceCommit, workspaceDirty := gitSnapshot(absWorkspace)
	now := time.Now().UTC()
	r := &Review{
		Version: 1, CreatedAt: now, UpdatedAt: now,
		TargetPath: absTarget, TargetCommit: targetCommit, TargetDirty: targetDirty,
		WorkspacePath: absWorkspace, WorkspaceCommit: workspaceCommit, WorkspaceDirty: workspaceDirty,
		OutputPath: absOut, Model: model, Tasks: BuildTasks(),
	}
	if err := os.MkdirAll(filepath.Join(absOut, "runs"), 0o755); err != nil {
		return nil, err
	}
	if err := Save(absOut, r); err != nil {
		return nil, err
	}
	return r, MaterializePrompts(absOut, r)
}

func Load(out string) (*Review, error) {
	b, err := os.ReadFile(filepath.Join(out, stateFile))
	if err != nil {
		return nil, err
	}
	var r Review
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func Save(out string, r *Review) error {
	r.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(out, stateFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func FindTask(r *Review, id string) (*Task, error) {
	for i := range r.Tasks {
		if r.Tasks[i].ID == id {
			return &r.Tasks[i], nil
		}
	}
	return nil, fmt.Errorf("unknown task %q", id)
}

func gitSnapshot(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	cmd := exec.Command("git", "-C", path, "rev-parse", "HEAD")
	b, err := cmd.Output()
	if err != nil {
		return "", false
	}
	commit := strings.TrimSpace(string(b))
	status := exec.Command("git", "-C", path, "status", "--porcelain")
	sb, err := status.Output()
	if err != nil {
		return commit, false
	}
	return commit, len(strings.TrimSpace(string(sb))) > 0
}
