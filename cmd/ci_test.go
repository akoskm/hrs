package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCITestCommandTriggersGitHubWorkflow(t *testing.T) {
	resetCICommandState()
	defer resetCICommandState()

	currentGitBranch = func(context.Context) (string, error) {
		return "feat/demo", nil
	}
	runExternalCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "gh" {
			t.Fatalf("command name = %q, want gh", name)
		}
		got := strings.Join(args, " ")
		want := "workflow run test.yml --ref feat/demo"
		if got != want {
			t.Fatalf("command args = %q, want %q", got, want)
		}
		return []byte(""), nil
	}

	out := &bytes.Buffer{}
	rootCmd.SetOut(out)
	rootCmd.SetErr(out)
	rootCmd.SetArgs([]string{"ci", "test"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ci test execute error = %v", err)
	}
	if !strings.Contains(out.String(), "Triggered GitHub tests for feat/demo") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCITestCommandReturnsGitHubCLIOutputOnFailure(t *testing.T) {
	resetCICommandState()
	defer resetCICommandState()

	currentGitBranch = func(context.Context) (string, error) {
		return "main", nil
	}
	runExternalCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("authentication required"), context.DeadlineExceeded
	}

	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
	rootCmd.SetArgs([]string{"ci", "test"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "authentication required") {
		t.Fatalf("error = %q", err)
	}
}

func resetCICommandState() {
	currentGitBranch = defaultCurrentGitBranch
	runExternalCommand = defaultRunExternalCommand
}
