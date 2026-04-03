package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncCodexCommand(t *testing.T) {
	resetProjectCommandState()
	dbPath = t.TempDir() + "/hrs.db"
	fixturesPath = ""

	projectOut := &bytes.Buffer{}
	rootCmd.SetOut(projectOut)
	rootCmd.SetErr(projectOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "add", "wrkpad", "--code", "wrkpad"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project add execute error = %v", err)
	}

	pathOut := &bytes.Buffer{}
	rootCmd.SetOut(pathOut)
	rootCmd.SetErr(pathOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "path", "add", "/Users/akoskm/Projects/wrkpad", "wrkpad"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("path add execute error = %v", err)
	}

	syncOut := &bytes.Buffer{}
	rootCmd.SetOut(syncOut)
	rootCmd.SetErr(syncOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "sync", "codex", "--path", filepath.Join("..", "testdata", "codex-sessions")})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("sync codex execute error = %v", err)
	}
	if strings.Contains(syncOut.String(), "Error") {
		t.Fatalf("unexpected output = %q", syncOut.String())
	}
}
