package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Antonio7098/ultraplan-architecture-gauntlet/internal/gauntlet"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = cmdInit(os.Args[2:])
	case "status":
		err = cmdStatus(os.Args[2:])
	case "next":
		err = cmdNext(os.Args[2:])
	case "prompt":
		err = cmdPrompt(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "index":
		err = cmdIndex(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`archgauntlet — one-off UltraPlan architecture review orchestrator

Usage:
  archgauntlet init   --target PATH [--workspace PATH] [--out .archgauntlet] [--model opencode-go/ox-alpha-free]
  archgauntlet status [--out .archgauntlet]
  archgauntlet next   [--out .archgauntlet]
  archgauntlet prompt --id TASK [--out .archgauntlet]
  archgauntlet run    --stage STAGE [--parallel 4] [--runner opencode|fake] [--model MODEL] [--project-dir PATH] [--dry-run] [--retry]
  archgauntlet index  [--out .archgauntlet]

Stages:
  scout generalist specialist failure change challenge chair synth arbiter
`)
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	target := fs.String("target", "", "implementation repo")
	workspace := fs.String("workspace", "", "planning workspace")
	out := fs.String("out", ".archgauntlet", "run directory")
	model := fs.String("model", "opencode-go/ox-alpha-free", "model")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := gauntlet.Init(*out, *target, *workspace, *model)
	if err != nil {
		return err
	}
	fmt.Printf("initialized %d tasks in %s\n", len(r.Tasks), *out)
	return nil
}
func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	out := fs.String("out", ".archgauntlet", "run directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := gauntlet.Load(*out)
	if err != nil {
		return err
	}
	fmt.Print(gauntlet.StatusText(r))
	return nil
}
func cmdNext(args []string) error {
	fs := flag.NewFlagSet("next", flag.ContinueOnError)
	out := fs.String("out", ".archgauntlet", "run directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := gauntlet.Load(*out)
	if err != nil {
		return err
	}
	fmt.Println(gauntlet.NextStage(r))
	return nil
}
func cmdPrompt(args []string) error {
	fs := flag.NewFlagSet("prompt", flag.ContinueOnError)
	out := fs.String("out", ".archgauntlet", "run directory")
	id := fs.String("id", "", "task id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := gauntlet.Load(*out)
	if err != nil {
		return err
	}
	t, err := gauntlet.FindTask(r, *id)
	if err != nil {
		return err
	}
	fmt.Print(gauntlet.RenderPrompt(r, t))
	return nil
}
func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	out := fs.String("out", ".archgauntlet", "run directory")
	stage := fs.String("stage", "", "stage")
	parallel := fs.Int("parallel", 4, "parallel jobs")
	runner := fs.String("runner", "opencode", "runner executable or fake")
	model := fs.String("model", "", "model override")
	dry := fs.Bool("dry-run", false, "print commands only")
	retry := fs.Bool("retry", false, "retry failed tasks")
	projectDir := fs.String("project-dir", "", "OpenCode project containing .opencode/agents")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := gauntlet.Load(*out)
	if err != nil {
		return err
	}
	return gauntlet.RunStage(context.Background(), r, gauntlet.RunOptions{Out: *out, Stage: *stage, Parallel: *parallel, Runner: *runner, Model: *model, DryRun: *dry, ProjectDir: *projectDir, Retry: *retry})
}
func cmdIndex(args []string) error {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	out := fs.String("out", ".archgauntlet", "run directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := gauntlet.Load(*out)
	if err != nil {
		return err
	}
	p, err := gauntlet.BuildIndex(*out, r)
	if err != nil {
		return err
	}
	abs, _ := filepath.Abs(p)
	fmt.Println(abs)
	return nil
}
