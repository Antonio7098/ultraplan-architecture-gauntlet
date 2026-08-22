package gauntlet

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitMaterializesPrompts(t *testing.T) {
	target := t.TempDir()
	workspace := t.TempDir()
	out := filepath.Join(t.TempDir(), "run")
	r, err := Init(out, target, workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Model != "openrouter/stealth/ox-alpha" {
		t.Fatalf("model %s", r.Model)
	}
	b, err := os.ReadFile(filepath.Join(out, "runs", "generalist-01", "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "Use subagents aggressively") {
		t.Fatalf("broad task missing delegation instruction")
	}
	b, _ = os.ReadFile(filepath.Join(out, "runs", "specialist-01a", "prompt.md"))
	if strings.Contains(string(b), "Use subagents aggressively") {
		t.Fatalf("narrow task should not receive broad delegation instruction")
	}
}

func TestInitCapturesGitSnapshotAndOutputPath(t *testing.T) {
	target := t.TempDir()
	workspace := t.TempDir()
	for _, dir := range []string{target, workspace} {
		cmds := [][]string{{"git", "init", "-q", "-b", "main"}, {"git", "config", "user.name", "Test"}, {"git", "config", "user.email", "test@example.invalid"}}
		for _, args := range cmds {
			c := exec.Command(args[0], args[1:]...)
			c.Dir = dir
			if out, err := c.CombinedOutput(); err != nil {
				t.Fatalf("%v: %v %s", args, err, out)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("fixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"git", "add", "."}, {"git", "commit", "-q", "-m", "fixture"}} {
			c := exec.Command(args[0], args[1:]...)
			c.Dir = dir
			if out, err := c.CombinedOutput(); err != nil {
				t.Fatalf("%v: %v %s", args, err, out)
			}
		}
	}
	out := filepath.Join(t.TempDir(), "review")
	r, err := Init(out, target, workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.TargetCommit == "" || r.WorkspaceCommit == "" {
		t.Fatalf("missing frozen commits: %#v", r)
	}
	absOut, _ := filepath.Abs(out)
	if r.OutputPath != absOut {
		t.Fatalf("output=%q want %q", r.OutputPath, absOut)
	}
}
