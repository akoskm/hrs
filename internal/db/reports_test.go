package db

import (
	"context"
	"testing"
	"time"
)

func TestRangeReportSplitsCrossMidnightEntries(t *testing.T) {
	t.Parallel()

	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	if _, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "late work",
		StartedAt:    time.Date(2026, 4, 2, 23, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 3, 1, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	report, err := store.RangeReport(ctx, time.Date(2026, 4, 2, 0, 0, 0, 0, time.Local), time.Date(2026, 4, 4, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("RangeReport() error = %v", err)
	}
	if len(report.Days) != 2 {
		t.Fatalf("len(report.Days) = %d, want 2", len(report.Days))
	}
	if report.Days[0].Date != "2026-04-02" || report.Days[0].TotalSecs != 3600 {
		t.Fatalf("first day = %#v, want 2026-04-02 with 3600 secs", report.Days[0])
	}
	if report.Days[1].Date != "2026-04-03" || report.Days[1].TotalSecs != 3600 {
		t.Fatalf("second day = %#v, want 2026-04-03 with 3600 secs", report.Days[1])
	}
}

func TestRangeReportIncludesUnassignedEntries(t *testing.T) {
	t.Parallel()

	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	if _, err := store.UpsertAgentEntry(ctx, AgentEntryUpsertInput{
		Description: "triage",
		StartedAt:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.Local),
		EndedAt:     time.Date(2026, 4, 2, 11, 30, 0, 0, time.Local),
		Operator:    "claude-code",
		SourceRef:   "session-1",
	}); err != nil {
		t.Fatalf("UpsertAgentEntry() error = %v", err)
	}

	report, err := store.RangeReport(ctx, time.Date(2026, 4, 2, 0, 0, 0, 0, time.Local), time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("RangeReport() error = %v", err)
	}
	if len(report.Projects) != 1 {
		t.Fatalf("len(report.Projects) = %d, want 1", len(report.Projects))
	}
	if report.Projects[0].ProjectName != "unassigned" {
		t.Fatalf("project name = %q, want unassigned", report.Projects[0].ProjectName)
	}
	if report.Projects[0].TotalSecs != 5400 {
		t.Fatalf("total_secs = %d, want 5400", report.Projects[0].TotalSecs)
	}
}

func TestRangeReportSeparatesBillableTotals(t *testing.T) {
	t.Parallel()

	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	if _, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "billable work",
		StartedAt:    time.Date(2026, 4, 2, 9, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 2, 11, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	falseVal := false
	if _, err := store.CreateManualEntry(ctx, ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "internal sync",
		StartedAt:    time.Date(2026, 4, 2, 14, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 2, 15, 30, 0, 0, time.Local),
		Billable:     &falseVal,
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	report, err := store.RangeReport(ctx, time.Date(2026, 4, 2, 0, 0, 0, 0, time.Local), time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("RangeReport() error = %v", err)
	}
	if report.Summary.BillableSecs != 7200 {
		t.Fatalf("summary billable_secs = %d, want 7200", report.Summary.BillableSecs)
	}
	if report.Summary.NonBillableSecs != 5400 {
		t.Fatalf("summary non_billable_secs = %d, want 5400", report.Summary.NonBillableSecs)
	}
	if report.Projects[0].BillableSecs != 7200 || report.Projects[0].NonBillableSecs != 5400 {
		t.Fatalf("project row = %#v, want billable 7200 and non-billable 5400", report.Projects[0])
	}
}

func TestRangeReportKeepsArchivedProjectMetadata(t *testing.T) {
	t.Parallel()

	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	project, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "billable work",
		StartedAt:    time.Date(2026, 4, 2, 9, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 2, 11, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if err := store.ArchiveProjectByID(ctx, project.ID); err != nil {
		t.Fatalf("ArchiveProjectByID() error = %v", err)
	}

	report, err := store.RangeReport(ctx, time.Date(2026, 4, 2, 0, 0, 0, 0, time.Local), time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("RangeReport() error = %v", err)
	}
	if len(report.Projects) != 1 {
		t.Fatalf("len(report.Projects) = %d, want 1", len(report.Projects))
	}
	if report.Projects[0].ProjectName != "Elaiia" {
		t.Fatalf("project name = %q, want Elaiia", report.Projects[0].ProjectName)
	}
	if report.Projects[0].Currency != "CHF" {
		t.Fatalf("currency = %q, want CHF", report.Projects[0].Currency)
	}
	if report.Projects[0].HourlyRate != 15000 {
		t.Fatalf("hourly_rate = %d, want 15000", report.Projects[0].HourlyRate)
	}
}

func TestRangeReportSeparatesRealUnassignedProjectFromUnassignedEntries(t *testing.T) {
	t.Parallel()

	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	if _, err := store.CreateProject(ctx, ProjectCreateInput{Name: "unassigned", Code: "unassigned-project", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, ManualEntryInput{
		ProjectIdent: "unassigned-project",
		Description:  "assigned work",
		StartedAt:    time.Date(2026, 4, 2, 9, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 2, 10, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.UpsertAgentEntry(ctx, AgentEntryUpsertInput{
		Description: "draft work",
		StartedAt:   time.Date(2026, 4, 2, 11, 0, 0, 0, time.Local),
		EndedAt:     time.Date(2026, 4, 2, 12, 30, 0, 0, time.Local),
		Operator:    "claude-code",
		SourceRef:   "session-2",
	}); err != nil {
		t.Fatalf("UpsertAgentEntry() error = %v", err)
	}

	report, err := store.RangeReport(ctx, time.Date(2026, 4, 2, 0, 0, 0, 0, time.Local), time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("RangeReport() error = %v", err)
	}
	if len(report.Projects) != 2 {
		t.Fatalf("len(report.Projects) = %d, want 2", len(report.Projects))
	}

	foundAssigned := false
	foundDraft := false
	for _, project := range report.Projects {
		if project.ProjectCode != nil && *project.ProjectCode == "unassigned-project" {
			foundAssigned = true
			if project.TotalSecs != 3600 {
				t.Fatalf("assigned project total_secs = %d, want 3600", project.TotalSecs)
			}
			continue
		}
		if project.ProjectCode == nil && project.ProjectName == "unassigned" {
			foundDraft = true
			if project.TotalSecs != 5400 {
				t.Fatalf("draft project total_secs = %d, want 5400", project.TotalSecs)
			}
		}
	}
	if !foundAssigned {
		t.Fatal("assigned project bucket not found")
	}
	if !foundDraft {
		t.Fatal("draft unassigned bucket not found")
	}
}

func TestRangeReportKeepsArchivedProjectOutOfUnassignedBucket(t *testing.T) {
	t.Parallel()

	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	project, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Archived Client", Code: "archived-client", Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, ManualEntryInput{
		ProjectIdent: "archived-client",
		Description:  "archived work",
		StartedAt:    time.Date(2026, 4, 2, 13, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 2, 14, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if err := store.ArchiveProjectByID(ctx, project.ID); err != nil {
		t.Fatalf("ArchiveProjectByID() error = %v", err)
	}

	report, err := store.RangeReport(ctx, time.Date(2026, 4, 2, 0, 0, 0, 0, time.Local), time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("RangeReport() error = %v", err)
	}
	if len(report.Projects) != 1 {
		t.Fatalf("len(report.Projects) = %d, want 1", len(report.Projects))
	}
	if report.Projects[0].ProjectName != "Archived Client" {
		t.Fatalf("project name = %q, want Archived Client", report.Projects[0].ProjectName)
	}
	if report.Projects[0].ProjectCode == nil || *report.Projects[0].ProjectCode != "archived-client" {
		t.Fatalf("project code = %v, want archived-client", report.Projects[0].ProjectCode)
	}
}
