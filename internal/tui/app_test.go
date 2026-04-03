package tui

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/sync"
)

func TestTimelineRendersAndAssigns(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := sync.ImportClaudeFixtures(ctx, store, filepath.Join("..", "..", "testdata", "claude-sessions")); err != nil {
		t.Fatalf("ImportClaudeFixtures() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}

	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(120, 30))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		out := string(b)
		return strings.Contains(out, "Description") && strings.Contains(out, "Refactor the auth module to use OAuth2") && strings.Contains(out, "draft")
	}, teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Assign Project")
	}, teatest.WithDuration(5*time.Second))
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		out := string(b)
		return strings.Contains(out, "confirmed") && strings.Contains(out, project.Name)
	}, teatest.WithDuration(5*time.Second))
	tm.Quit()

	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no entries after assignment")
	}
	if entries[0].Status != "confirmed" {
		t.Fatalf("status = %q, want confirmed", entries[0].Status)
	}
	if entries[0].ProjectID == nil || *entries[0].ProjectID != project.ID {
		t.Fatalf("project_id = %v, want %q", entries[0].ProjectID, project.ID)
	}
}

func TestTimelineQuitsOnQ(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := sync.ImportClaudeFixtures(ctx, store, filepath.Join("..", "..", "testdata", "claude-sessions")); err != nil {
		t.Fatalf("ImportClaudeFixtures() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}

	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(120, 30))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Timeline")
	}, teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

func TestAssignPickerShowsProjectsFromDB(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Delta Labs", Code: "delta", HourlyRate: 12000, Currency: "EUR"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := sync.ImportClaudeFixtures(ctx, store, filepath.Join("..", "..", "testdata", "claude-sessions")); err != nil {
		t.Fatalf("ImportClaudeFixtures() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("updated model type = %T", updated)
	}
	view := app.View()
	if !strings.Contains(view, "Assign Project") {
		t.Fatalf("view missing assign picker: %q", view)
	}
	if !strings.Contains(view, "Delta Labs (delta)") || !strings.Contains(view, "Elaiia (elaiia)") {
		t.Fatalf("view missing DB projects: %q", view)
	}
}

func TestTimelineTruncatesLongDescriptionToWidth(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "This is a deliberately long description that should not overflow a narrow terminal view in the timeline",
		StartedAt:    time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	app, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("updated model type = %T", updated)
	}
	view := app.View()
	if !strings.Contains(view, "...") {
		t.Fatalf("view missing truncation: %q", view)
	}
	if !strings.Contains(view, "Time") || !strings.Contains(view, "Status") {
		t.Fatalf("view missing table header: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if len(line) > 40 {
			t.Fatalf("line too wide (%d): %q", len(line), line)
		}
	}
}

func TestTimelineScrollsToKeepCursorVisible(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	for i := 0; i < 12; i++ {
		_, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
			ProjectIdent: "elaiia",
			Description:  "Entry " + strconv.Itoa(i),
			StartedAt:    time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour),
			EndedAt:      time.Date(2026, 4, 3, 9, 30, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour),
		})
		if err != nil {
			t.Fatalf("CreateManualEntry() error = %v", err)
		}
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	app := updated.(AppModel)
	for i := 0; i < 10; i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
		app = updated.(AppModel)
	}
	view := app.View()
	if strings.Contains(view, "Entry 0") {
		t.Fatalf("view should have scrolled past first row: %q", view)
	}
	if !strings.Contains(view, "Entry 10") {
		t.Fatalf("view missing selected later row: %q", view)
	}
}

func TestTimelineHomeEndAndPageKeys(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	for i := 0; i < 20; i++ {
		_, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
			ProjectIdent: "elaiia",
			Description:  "Entry " + strconv.Itoa(i),
			StartedAt:    time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour),
			EndedAt:      time.Date(2026, 4, 3, 9, 30, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour),
		})
		if err != nil {
			t.Fatalf("CreateManualEntry() error = %v", err)
		}
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	app := updated.(AppModel)

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnd})
	app = updated.(AppModel)
	if app.cursor != 19 {
		t.Fatalf("cursor after end = %d, want 19", app.cursor)
	}
	if !strings.Contains(app.View(), "Entry 19") {
		t.Fatalf("view missing last entry after end: %q", app.View())
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	app = updated.(AppModel)
	if app.cursor >= 19 {
		t.Fatalf("cursor after pgup = %d, want less than 19", app.cursor)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyHome})
	app = updated.(AppModel)
	if app.cursor != 0 {
		t.Fatalf("cursor after home = %d, want 0", app.cursor)
	}
	if !strings.Contains(app.View(), "Entry 0") {
		t.Fatalf("view missing first entry after home: %q", app.View())
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	app = updated.(AppModel)
	if app.cursor == 0 {
		t.Fatal("cursor did not move on pgdown")
	}
}

func openTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	return store
}
