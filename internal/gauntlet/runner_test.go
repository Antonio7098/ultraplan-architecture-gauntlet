package gauntlet

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFakeRunnerAndStageGate(t *testing.T) {
	target := t.TempDir()
	out := filepath.Join(t.TempDir(), "run")
	r, err := Init(out, target, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := RunStage(context.Background(), r, RunOptions{Out: out, Stage: "scout", Parallel: 3, Runner: "fake"}); err != nil {
		t.Fatal(err)
	}
	if got := NextStage(r); got != "generalist" {
		t.Fatalf("next=%s", got)
	}
	if err := RunStage(context.Background(), r, RunOptions{Out: out, Stage: "challenge", Parallel: 1, Runner: "fake"}); err == nil {
		t.Fatalf("expected gate failure")
	}
}
