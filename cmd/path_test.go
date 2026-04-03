package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestPathAdd(t *testing.T) {
	resetProjectCommandState()
	dbPath = t.TempDir() + "/hrs.db"
	fixturesPath = ""

	projectOut := &bytes.Buffer{}
	rootCmd.SetOut(projectOut)
	rootCmd.SetErr(projectOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "add", "Elaiia", "--code", "elaiia"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project add execute error = %v", err)
	}

	pathOut := &bytes.Buffer{}
	rootCmd.SetOut(pathOut)
	rootCmd.SetErr(pathOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "path", "add", "/Users/akos/code/elaiia", "elaiia"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("path add execute error = %v", err)
	}
	if !strings.Contains(pathOut.String(), "/Users/akos/code/elaiia") {
		t.Fatalf("path add output = %q", pathOut.String())
	}
}
