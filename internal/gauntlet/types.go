package gauntlet

import "time"

type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusRunning   TaskStatus = "running"
	StatusSucceeded TaskStatus = "succeeded"
	StatusFailed    TaskStatus = "failed"
)

type Task struct {
	ID          string     `json:"id"`
	Stage       string     `json:"stage"`
	Title       string     `json:"title"`
	Role        string     `json:"role"`
	Focus       string     `json:"focus"`
	Broad       bool       `json:"broad"`
	Status      TaskStatus `json:"status"`
	Attempts    int        `json:"attempts"`
	LastError   string     `json:"last_error,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type Review struct {
	Version         int       `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	TargetPath      string    `json:"target_path"`
	TargetCommit    string    `json:"target_commit,omitempty"`
	TargetDirty     bool      `json:"target_dirty,omitempty"`
	WorkspacePath   string    `json:"workspace_path"`
	WorkspaceCommit string    `json:"workspace_commit,omitempty"`
	WorkspaceDirty  bool      `json:"workspace_dirty,omitempty"`
	OutputPath      string    `json:"output_path"`
	Model           string    `json:"model"`
	Tasks           []Task    `json:"tasks"`
}
