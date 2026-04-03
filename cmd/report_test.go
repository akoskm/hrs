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
	rootCmd.SetArgs([]string{"--db", dbPath, "report", "--week"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("report week execute error = %v", err)
	}
	if !strings.Contains(tableOut.String(), "Elaiia") || !strings.Contains(tableOut.String(), "CHF") {
		t.Fatalf("table output = %q", tableOut.String())
	}

	jsonOut := &bytes.Buffer{}
	rootCmd.SetOut(jsonOut)
	rootCmd.SetErr(jsonOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "report", "--week", "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("report week json execute error = %v", err)
	}
	if !strings.Contains(jsonOut.String(), "\"ProjectName\"") && !strings.Contains(jsonOut.String(), "\"projectName\"") && !strings.Contains(jsonOut.String(), "Elaiia") {
		t.Fatalf("json output = %q", jsonOut.String())
	}
}
