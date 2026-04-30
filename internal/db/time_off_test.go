package db

import (
	"context"
	"testing"

	"github.com/akoskm/hrs/internal/model"
)

func TestEnsureProjectDefaultTimeOffTypesSeedsPerProject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	alpha, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: model.CurrencyUSD})
	if err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	beta, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Beta", Code: "beta", Currency: model.CurrencyUSD})
	if err != nil {
		t.Fatalf("CreateProject(beta) error = %v", err)
	}

	if err := store.EnsureProjectDefaultTimeOffTypes(ctx, alpha.ID); err != nil {
		t.Fatalf("EnsureProjectDefaultTimeOffTypes(alpha) error = %v", err)
	}
	if err := store.EnsureProjectDefaultTimeOffTypes(ctx, alpha.ID); err != nil {
		t.Fatalf("EnsureProjectDefaultTimeOffTypes(alpha) second run error = %v", err)
	}

	alphaTypes, err := store.ListTimeOffTypesByProject(ctx, alpha.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject(alpha) error = %v", err)
	}
	if len(alphaTypes) != 3 {
		t.Fatalf("len(alphaTypes) = %d, want 3", len(alphaTypes))
	}
	for i, want := range []string{"Holiday", "Sick Leave", "Vacation"} {
		if alphaTypes[i].Name != want {
			t.Fatalf("alphaTypes[%d].Name = %q, want %q", i, alphaTypes[i].Name, want)
		}
		if alphaTypes[i].ProjectID != alpha.ID {
			t.Fatalf("alphaTypes[%d].ProjectID = %q, want %q", i, alphaTypes[i].ProjectID, alpha.ID)
		}
		if !alphaTypes[i].IsSystem {
			t.Fatalf("alphaTypes[%d].IsSystem = false, want true", i)
		}
	}

	betaTypes, err := store.ListTimeOffTypesByProject(ctx, beta.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject(beta) error = %v", err)
	}
	if len(betaTypes) != 0 {
		t.Fatalf("len(betaTypes) = %d, want 0 before seeding", len(betaTypes))
	}

	if err := store.EnsureProjectDefaultTimeOffTypes(ctx, beta.ID); err != nil {
		t.Fatalf("EnsureProjectDefaultTimeOffTypes(beta) error = %v", err)
	}
	betaTypes, err = store.ListTimeOffTypesByProject(ctx, beta.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject(beta) after seed error = %v", err)
	}
	if len(betaTypes) != 3 {
		t.Fatalf("len(betaTypes) = %d, want 3", len(betaTypes))
	}
	if betaTypes[0].ProjectID != beta.ID {
		t.Fatalf("betaTypes[0].ProjectID = %q, want %q", betaTypes[0].ProjectID, beta.ID)
	}
}

func TestTimeOffDaysAreScopedPerProjectAndDate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	alpha, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: model.CurrencyUSD})
	if err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	beta, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Beta", Code: "beta", Currency: model.CurrencyUSD})
	if err != nil {
		t.Fatalf("CreateProject(beta) error = %v", err)
	}
	if err := store.EnsureProjectDefaultTimeOffTypes(ctx, alpha.ID); err != nil {
		t.Fatalf("EnsureProjectDefaultTimeOffTypes(alpha) error = %v", err)
	}
	if err := store.EnsureProjectDefaultTimeOffTypes(ctx, beta.ID); err != nil {
		t.Fatalf("EnsureProjectDefaultTimeOffTypes(beta) error = %v", err)
	}

	alphaCustom, err := store.CreateTimeOffType(ctx, alpha.ID, "Conference Leave")
	if err != nil {
		t.Fatalf("CreateTimeOffType(alpha) error = %v", err)
	}
	alphaTypes, err := store.ListTimeOffTypesByProject(ctx, alpha.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject(alpha) error = %v", err)
	}
	if len(alphaTypes) != 4 {
		t.Fatalf("len(alphaTypes) = %d, want 4", len(alphaTypes))
	}

	betaTypes, err := store.ListTimeOffTypesByProject(ctx, beta.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject(beta) error = %v", err)
	}
	if len(betaTypes) != 3 {
		t.Fatalf("len(betaTypes) = %d, want 3", len(betaTypes))
	}

	day := "2026-04-14"
	if _, err := store.UpsertTimeOffDay(ctx, alpha.ID, alphaCustom.ID, day); err != nil {
		t.Fatalf("UpsertTimeOffDay(alpha) error = %v", err)
	}
	if _, err := store.UpsertTimeOffDay(ctx, beta.ID, betaTypes[0].ID, day); err != nil {
		t.Fatalf("UpsertTimeOffDay(beta) error = %v", err)
	}
	if _, err := store.UpsertTimeOffDay(ctx, alpha.ID, alphaTypes[0].ID, day); err != nil {
		t.Fatalf("UpsertTimeOffDay(alpha overwrite) error = %v", err)
	}

	records, err := store.ListTimeOffDaysInRange(ctx, "2026-04-01", "2026-04-30")
	if err != nil {
		t.Fatalf("ListTimeOffDaysInRange() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[0].ProjectID == records[1].ProjectID {
		t.Fatalf("records project IDs = %q and %q, want different projects", records[0].ProjectID, records[1].ProjectID)
	}
	if records[0].Day != day || records[1].Day != day {
		t.Fatalf("record days = %q and %q, want %q", records[0].Day, records[1].Day, day)
	}

	var alphaRecord model.TimeOffDayDetail
	for _, record := range records {
		if record.ProjectID == alpha.ID {
			alphaRecord = record
			break
		}
	}
	if alphaRecord.TimeOffType != "Conference Leave" && alphaRecord.TimeOffType != "Holiday" {
		t.Fatalf("alphaRecord.TimeOffType = %q, want seeded or custom label", alphaRecord.TimeOffType)
	}
	if alphaRecord.TimeOffType != alphaTypes[0].Name {
		t.Fatalf("alphaRecord.TimeOffType = %q, want overwritten type %q", alphaRecord.TimeOffType, alphaTypes[0].Name)
	}

	if err := store.DeleteTimeOffDay(ctx, alpha.ID, day); err != nil {
		t.Fatalf("DeleteTimeOffDay(alpha) error = %v", err)
	}
	records, err = store.ListTimeOffDaysInRange(ctx, "2026-04-01", "2026-04-30")
	if err != nil {
		t.Fatalf("ListTimeOffDaysInRange() after delete error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) after delete = %d, want 1", len(records))
	}
	if records[0].ProjectID != beta.ID {
		t.Fatalf("remaining ProjectID = %q, want %q", records[0].ProjectID, beta.ID)
	}
}

func TestTimeOffAllowanceUpsertAndSummary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	project, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: model.CurrencyUSD})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := store.EnsureProjectDefaultTimeOffTypes(ctx, project.ID); err != nil {
		t.Fatalf("EnsureProjectDefaultTimeOffTypes() error = %v", err)
	}
	types, err := store.ListTimeOffTypesByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject() error = %v", err)
	}
	var vacation model.TimeOffType
	for _, item := range types {
		if item.Name == "Vacation" {
			vacation = item
			break
		}
	}
	if vacation.ID == "" {
		t.Fatal("vacation type not found")
	}

	allowance, err := store.UpsertTimeOffAllowance(ctx, project.ID, vacation.ID, 2026, 20)
	if err != nil {
		t.Fatalf("UpsertTimeOffAllowance() error = %v", err)
	}
	if allowance.AllowedDays != 20 {
		t.Fatalf("allowance.AllowedDays = %d, want 20", allowance.AllowedDays)
	}
	allowance, err = store.UpsertTimeOffAllowance(ctx, project.ID, vacation.ID, 2026, 22)
	if err != nil {
		t.Fatalf("UpsertTimeOffAllowance() second call error = %v", err)
	}
	if allowance.AllowedDays != 22 {
		t.Fatalf("allowance.AllowedDays after update = %d, want 22", allowance.AllowedDays)
	}
	allowances, err := store.ListTimeOffAllowancesByProject(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("ListTimeOffAllowancesByProject() error = %v", err)
	}
	if len(allowances) != 1 {
		t.Fatalf("len(allowances) = %d, want 1", len(allowances))
	}
	if _, err := store.UpsertTimeOffDay(ctx, project.ID, vacation.ID, "2026-04-10"); err != nil {
		t.Fatalf("UpsertTimeOffDay() first error = %v", err)
	}
	if _, err := store.UpsertTimeOffDay(ctx, project.ID, vacation.ID, "2026-04-11"); err != nil {
		t.Fatalf("UpsertTimeOffDay() second error = %v", err)
	}
	summaries, err := store.ListTimeOffAllowanceSummaries(ctx, 2026)
	if err != nil {
		t.Fatalf("ListTimeOffAllowanceSummaries() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(summaries) = %d, want 1", len(summaries))
	}
	if summaries[0].UsedDays != 2 {
		t.Fatalf("UsedDays = %d, want 2", summaries[0].UsedDays)
	}
	if summaries[0].RemainingDays != 20 {
		t.Fatalf("RemainingDays = %d, want 20", summaries[0].RemainingDays)
	}
	if summaries[0].TimeOffType != "Vacation" {
		t.Fatalf("TimeOffType = %q, want Vacation", summaries[0].TimeOffType)
	}
	if summaries[0].ProjectID != project.ID {
		t.Fatalf("ProjectID = %q, want %q", summaries[0].ProjectID, project.ID)
	}
	if err := store.DeleteTimeOffAllowance(ctx, project.ID, vacation.ID, 2026); err != nil {
		t.Fatalf("DeleteTimeOffAllowance() error = %v", err)
	}
	allowances, err = store.ListTimeOffAllowancesByProject(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("ListTimeOffAllowancesByProject() after delete error = %v", err)
	}
	if len(allowances) != 0 {
		t.Fatalf("len(allowances) after delete = %d, want 0", len(allowances))
	}
}

func TestTimeOffAllowanceSummariesAreScopedByProjectTypeAndYear(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	alpha, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: model.CurrencyUSD})
	if err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	beta, err := store.CreateProject(ctx, ProjectCreateInput{Name: "Beta", Code: "beta", Currency: model.CurrencyUSD})
	if err != nil {
		t.Fatalf("CreateProject(beta) error = %v", err)
	}
	for _, projectID := range []string{alpha.ID, beta.ID} {
		if err := store.EnsureProjectDefaultTimeOffTypes(ctx, projectID); err != nil {
			t.Fatalf("EnsureProjectDefaultTimeOffTypes(%s) error = %v", projectID, err)
		}
	}
	alphaTypes, _ := store.ListTimeOffTypesByProject(ctx, alpha.ID)
	betaTypes, _ := store.ListTimeOffTypesByProject(ctx, beta.ID)
	alphaHoliday := alphaTypes[0]
	betaHoliday := betaTypes[0]
	if _, err := store.UpsertTimeOffAllowance(ctx, alpha.ID, alphaHoliday.ID, 2026, 11); err != nil {
		t.Fatalf("UpsertTimeOffAllowance(alpha 2026) error = %v", err)
	}
	if _, err := store.UpsertTimeOffAllowance(ctx, alpha.ID, alphaHoliday.ID, 2027, 12); err != nil {
		t.Fatalf("UpsertTimeOffAllowance(alpha 2027) error = %v", err)
	}
	if _, err := store.UpsertTimeOffAllowance(ctx, beta.ID, betaHoliday.ID, 2026, 9); err != nil {
		t.Fatalf("UpsertTimeOffAllowance(beta 2026) error = %v", err)
	}
	if _, err := store.UpsertTimeOffDay(ctx, alpha.ID, alphaHoliday.ID, "2026-05-01"); err != nil {
		t.Fatalf("UpsertTimeOffDay(alpha 2026) error = %v", err)
	}
	if _, err := store.UpsertTimeOffDay(ctx, alpha.ID, alphaHoliday.ID, "2027-05-01"); err != nil {
		t.Fatalf("UpsertTimeOffDay(alpha 2027) error = %v", err)
	}
	if _, err := store.UpsertTimeOffDay(ctx, beta.ID, betaHoliday.ID, "2026-06-01"); err != nil {
		t.Fatalf("UpsertTimeOffDay(beta 2026) error = %v", err)
	}
	summaries, err := store.ListTimeOffAllowanceSummaries(ctx, 2026)
	if err != nil {
		t.Fatalf("ListTimeOffAllowanceSummaries(2026) error = %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("len(summaries) = %d, want 2", len(summaries))
	}
	for _, summary := range summaries {
		if summary.Year != 2026 {
			t.Fatalf("summary.Year = %d, want 2026", summary.Year)
		}
		if summary.UsedDays != 1 {
			t.Fatalf("summary.UsedDays = %d, want 1", summary.UsedDays)
		}
	}
}
