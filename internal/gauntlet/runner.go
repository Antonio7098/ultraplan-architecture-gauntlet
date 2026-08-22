package gauntlet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type RunOptions struct {
	Out        string
	Stage      string
	Parallel   int
	Runner     string
	Model      string
	DryRun     bool
	ProjectDir string
	Retry      bool
}

func RunStage(ctx context.Context, r *Review, opts RunOptions) error {
	if opts.Parallel < 1 {
		opts.Parallel = 1
	}
	if opts.Runner == "" {
		opts.Runner = "opencode"
	}
	if opts.Model == "" {
		opts.Model = r.Model
	}
	if opts.ProjectDir == "" {
		opts.ProjectDir, _ = os.Getwd()
	}
	if err := stageReady(r, opts.Stage); err != nil {
		return err
	}
	var ids []string
	for i := range r.Tasks {
		t := &r.Tasks[i]
		if t.Stage != opts.Stage {
			continue
		}
		if t.Status == StatusSucceeded {
			continue
		}
		if t.Status == StatusFailed && !opts.Retry {
			continue
		}
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return nil
	}
	if opts.DryRun {
		for _, id := range ids {
			fmt.Printf("%s\n", commandPreview(r, id, opts))
		}
		return nil
	}
	jobs := make(chan string)
	errCh := make(chan error, len(ids))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := 0; i < opts.Parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				mu.Lock()
				t, _ := FindTask(r, id)
				now := time.Now().UTC()
				t.Status = StatusRunning
				t.Attempts++
				t.StartedAt = &now
				t.LastError = ""
				_ = Save(opts.Out, r)
				mu.Unlock()
				err := runOne(ctx, r, id, opts)
				mu.Lock()
				t, _ = FindTask(r, id)
				done := time.Now().UTC()
				t.CompletedAt = &done
				if err != nil {
					t.Status = StatusFailed
					t.LastError = err.Error()
					errCh <- fmt.Errorf("%s: %w", id, err)
				} else {
					t.Status = StatusSucceeded
				}
				_ = Save(opts.Out, r)
				mu.Unlock()
			}
		}()
	}
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)
	wg.Wait()
	close(errCh)
	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func runOne(ctx context.Context, r *Review, id string, opts RunOptions) error {
	dir := filepath.Join(opts.Out, "runs", id)
	prompt, err := os.ReadFile(filepath.Join(dir, "prompt.md"))
	if err != nil {
		return err
	}
	if opts.Runner == "fake" {
		return os.WriteFile(filepath.Join(dir, "stdout.md"), []byte("# Synthetic test result\n\nTask "+id+" executed successfully by fake runner.\n"), 0o644)
	}
	t, _ := FindTask(r, id)
	args := []string{"run", "--model", opts.Model, "--agent", t.Role, "--format", "default", "--dir", opts.ProjectDir, string(prompt)}
	cmd := exec.CommandContext(ctx, opts.Runner, args...)
	stdoutPath := filepath.Join(dir, "stdout.md")
	stderrPath := filepath.Join(dir, "stderr.log")
	stdout, err := os.Create(stdoutPath)
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.Create(stderrPath)
	if err != nil {
		return err
	}
	defer stderr.Close()
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

func commandPreview(r *Review, id string, opts RunOptions) string {
	t, _ := FindTask(r, id)
	return fmt.Sprintf("%s run --model %s --agent %s --format default --dir %s <prompt:%s>", opts.Runner, opts.Model, t.Role, opts.ProjectDir, id)
}

func stageReady(r *Review, stage string) error {
	valid := false
	idx := -1
	for i, s := range stages {
		if s == stage {
			valid = true
			idx = i
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown stage %q", stage)
	}
	if idx < 5 {
		return nil
	}
	for _, t := range r.Tasks {
		for j := 0; j < idx; j++ {
			if t.Stage == stages[j] && t.Status != StatusSucceeded {
				return fmt.Errorf("stage %s blocked: %s is %s", stage, t.ID, t.Status)
			}
		}
	}
	return nil
}
