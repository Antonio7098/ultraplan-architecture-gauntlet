package gauntlet

import "testing"

func TestBuildTasksCountAndIDs(t *testing.T) {
	tasks := BuildTasks()
	if len(tasks) != 94 {
		t.Fatalf("got %d tasks, want 94", len(tasks))
	}
	seen := map[string]bool{}
	for _, task := range tasks {
		if seen[task.ID] {
			t.Fatalf("duplicate %s", task.ID)
		}
		seen[task.ID] = true
	}
	if !seen["specialist-24b"] || !seen["arbiter-01"] {
		t.Fatalf("expected canonical tasks")
	}
}
