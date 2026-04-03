package tui

import (
	"context"
	"path/filepath"
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
		return strings.Contains(out, "Refactor the auth module to use OAuth2") && strings.Contains(out, "draft")
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

func openTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	return store
}
