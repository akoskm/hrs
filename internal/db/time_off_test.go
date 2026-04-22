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
