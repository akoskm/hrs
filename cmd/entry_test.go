package cmd

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/akoskm/hrs/internal/db"
)

func TestEntryUpsertCreatesAndUpdatesAgentEntry(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hrs.db")

	projectOut := new(bytes.Buffer)
	rootCmd.SetOut(projectOut)
	rootCmd.SetErr(projectOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "add", "hrs", "--code", "hrs"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project add error = %v, output=%s", err, projectOut.String())
	}

	upsertOut := new(bytes.Buffer)
	rootCmd.SetOut(upsertOut)
	rootCmd.SetErr(upsertOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "entry", "upsert", "--source", "opencode", "--source-ref", "ses-1", "--project", "hrs", "--from", "13:00", "--to", "14:00", "--description", "working on TUI"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("entry upsert create error = %v, output=%s", err, upsertOut.String())
	}

	upsertOut.Reset()
	rootCmd.SetOut(upsertOut)
	rootCmd.SetErr(upsertOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "entry", "upsert", "--source", "opencode", "--source-ref", "ses-1", "--project", "hrs", "--from", "13:00", "--to", "14:30", "--description", "working on inspector"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("entry upsert update error = %v, output=%s", err, upsertOut.String())
	}

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	entries, err := store.ListEntries(t.Context())
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Description == nil || *entries[0].Description != "working on inspector" {
		t.Fatalf("description = %v, want updated description", entries[0].Description)
	}
	if entries[0].EndedAt.Sub(entries[0].StartedAt) != 90*time.Minute {
		t.Fatalf("duration = %s, want 90m", entries[0].EndedAt.Sub(entries[0].StartedAt))
	}
}
