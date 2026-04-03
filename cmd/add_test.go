package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/akoskm/hrs/internal/db"
)

func TestAddManualEntry(t *testing.T) {
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

	addOut := &bytes.Buffer{}
	rootCmd.SetOut(addOut)
	rootCmd.SetErr(addOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "add", "--project", "elaiia", "--from", "09:00", "--to", "11:00", "Sprint planning"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add execute error = %v", err)
	}
	if !strings.Contains(addOut.String(), "Sprint planning") || !strings.Contains(addOut.String(), "confirmed") {
		t.Fatalf("add output = %q", addOut.String())
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
	if entries[0].Status != "confirmed" || entries[0].Operator != "human" {
		t.Fatalf("entry = %#v", entries[0])
	}
}

func TestAddManualEntryWithDate(t *testing.T) {
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

	addOut := &bytes.Buffer{}
	rootCmd.SetOut(addOut)
	rootCmd.SetErr(addOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "add", "--project", "elaiia", "--date", "2026-04-02", "--from", "14:00", "--to", "15:30", "Client call"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add execute error = %v", err)
	}

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	entries, err := store.ListEntries(context.Background())
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if got := entries[0].StartedAt.In(time.Local).Format("2006-01-02 15:04"); got != "2026-04-02 14:00" {
		t.Fatalf("started_at = %q, want %q", got, "2026-04-02 14:00")
	}
}

func TestAddManualEntryBillableOverride(t *testing.T) {
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

	addOut := &bytes.Buffer{}
	rootCmd.SetOut(addOut)
	rootCmd.SetErr(addOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "add", "--project", "elaiia", "--from", "09:00", "--to", "10:00", "--billable=false", "Internal sync"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add execute error = %v", err)
	}

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	entries, err := store.ListEntries(context.Background())
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Billable {
		t.Fatalf("billable = %t, want false", entries[0].Billable)
	}
}
