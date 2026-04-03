package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestClientAddAndList(t *testing.T) {
	dbPath = t.TempDir() + "/hrs.db"
	fixturesPath = ""

	addOut := &bytes.Buffer{}
	rootCmd.SetOut(addOut)
	rootCmd.SetErr(addOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "client", "add", "Delta Labs"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("client add execute error = %v", err)
	}
	if !strings.Contains(addOut.String(), "Delta Labs") {
		t.Fatalf("client add output = %q", addOut.String())
	}

	listOut := &bytes.Buffer{}
	rootCmd.SetOut(listOut)
	rootCmd.SetErr(listOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "client", "list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("client list execute error = %v", err)
	}
	if !strings.Contains(listOut.String(), "Delta Labs") {
		t.Fatalf("client list output = %q", listOut.String())
	}
}
