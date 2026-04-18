package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/akoskm/hrs/internal/db"
)

func TestReportWeekTableAndJSON(t *testing.T) {
	resetProjectCommandState()
	resetReportCommandState()
	dbPath = t.TempDir() + "/hrs.db"
	fixturesPath = ""
	reportNow = func() time.Time { return time.Date(2026, 4, 3, 10, 0, 0, 0, time.Local) }
	defer func() { reportNow = time.Now }()

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Sprint planning", StartedAt: time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	falseVal := false
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Client call", StartedAt: time.Date(2026, 4, 2, 14, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 2, 15, 30, 0, 0, time.UTC), Billable: &falseVal}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	tableOut := &bytes.Buffer{}
	rootCmd.SetOut(tableOut)
	rootCmd.SetErr(tableOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "report", "--from", "2026-04-01", "--to", "2026-04-07"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("report range execute error = %v", err)
	}
	if !strings.Contains(tableOut.String(), "Summary") || !strings.Contains(tableOut.String(), "By project") || !strings.Contains(tableOut.String(), "Elaiia") || !strings.Contains(tableOut.String(), "CHF") {
		t.Fatalf("table output = %q", tableOut.String())
	}
	if !strings.Contains(tableOut.String(), "By day") || !strings.Contains(tableOut.String(), "2026-04-02") {
		t.Fatalf("table output missing daily section = %q", tableOut.String())
	}

	resetReportCommandState()
	jsonOut := &bytes.Buffer{}
	rootCmd.SetOut(jsonOut)
	rootCmd.SetErr(jsonOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "report", "--week", "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("report week json execute error = %v", err)
	}
	if !strings.Contains(jsonOut.String(), "\"range\"") || !strings.Contains(jsonOut.String(), "\"summary\"") || !strings.Contains(jsonOut.String(), "\"projects\"") || !strings.Contains(jsonOut.String(), "\"days\"") || !strings.Contains(jsonOut.String(), "\"project_name\"") || !strings.Contains(jsonOut.String(), "Elaiia") {
		t.Fatalf("json output = %q", jsonOut.String())
	}
}

func TestReportDefaultsToCurrentWeekAndHandlesEmptyRange(t *testing.T) {
	resetProjectCommandState()
	resetReportCommandState()
	dbPath = t.TempDir() + "/hrs.db"
	fixturesPath = ""
	reportNow = func() time.Time { return time.Date(2026, 4, 3, 10, 0, 0, 0, time.Local) }
	defer func() { reportNow = time.Now }()

	out := &bytes.Buffer{}
	rootCmd.SetOut(out)
	rootCmd.SetErr(out)
	rootCmd.SetArgs([]string{"--db", dbPath, "report"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("default report execute error = %v", err)
	}
	if !strings.Contains(out.String(), "Range: 2026-03-30..2026-04-05") || !strings.Contains(out.String(), "By project") || !strings.Contains(out.String(), "By day") {
		t.Fatalf("default output = %q", out.String())
	}

	resetReportCommandState()
	jsonOut := &bytes.Buffer{}
	rootCmd.SetOut(jsonOut)
	rootCmd.SetErr(jsonOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "report", "--month", "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("month report execute error = %v", err)
	}
	if !strings.Contains(jsonOut.String(), "\"from\": \"2026-04-01\"") || !strings.Contains(jsonOut.String(), "\"to\": \"2026-04-30\"") || !strings.Contains(jsonOut.String(), "\"projects\": []") || !strings.Contains(jsonOut.String(), "\"days\": []") {
		t.Fatalf("month json output = %q", jsonOut.String())
	}
}

func resetReportCommandState() {
	reportWeek = false
	reportMonth = false
	reportYear = false
	reportJSON = false
	reportFrom = ""
	reportTo = ""
}
