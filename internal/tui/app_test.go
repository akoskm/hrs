package tui

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		out := stripANSI(string(b))
		return strings.Contains(out, "Refactor the auth module to use OAuth2") && strings.Contains(out, "draft")
	}, teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "Assign Project")
	}, teatest.WithDuration(5*time.Second))
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		out := stripANSI(string(b))
		return strings.Contains(out, "confirmed") && strings.Contains(out, project.Name)
	}, teatest.WithDuration(5*time.Second))
	tm.Quit()

	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	confirmed := 0
	for _, entry := range entries {
		if entry.ProjectID != nil && *entry.ProjectID == project.ID && entry.Status == "confirmed" {
			confirmed++
		}
	}
	if confirmed == 0 {
		t.Fatalf("no confirmed entry assigned to %q: %#v", project.ID, entries)
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
	view := stripANSI(app.View())
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
	view := stripANSI(app.View())
	if !strings.Contains(view, "...") {
		t.Fatalf("view missing truncation: %q", view)
	}
	if !strings.Contains(view, "Time") || !strings.Contains(view, "Status") {
		t.Fatalf("view missing table header: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 40 {
			t.Fatalf("line too wide (%d): %q", lipgloss.Width(line), line)
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
	view := stripANSI(app.View())
	if strings.Contains(view, "Entry 0") {
		t.Fatalf("view should have scrolled past first row: %q", view)
	}
	if !strings.Contains(view, "Entry 1") {
		t.Fatalf("view missing expected later row: %q", view)
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
	if !strings.Contains(stripANSI(app.View()), "Entry 0") {
		t.Fatalf("view missing oldest entry after end: %q", stripANSI(app.View()))
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
	if !strings.Contains(stripANSI(app.View()), "Entry 19") {
		t.Fatalf("view missing newest entry after home: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	app = updated.(AppModel)
	if app.cursor == 0 {
		t.Fatal("cursor did not move on pgdown")
	}
}

func TestTimelineGroupsByDateNewestFirst(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	_, _ = store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Older", StartedAt: time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)})
	_, _ = store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Newer", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)})

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	app := updated.(AppModel)
	view := stripANSI(app.View())
	newerHeader := strings.Index(view, "── 2026-04-03")
	olderHeader := strings.Index(view, "── 2026-04-02")
	newerEntry := strings.Index(view, "Newer")
	olderEntry := strings.Index(view, "Older")
	if newerHeader == -1 || olderHeader == -1 || newerEntry == -1 || olderEntry == -1 {
		t.Fatalf("missing grouped content: %q", view)
	}
	if !(newerHeader < newerEntry && newerEntry < olderHeader && olderHeader < olderEntry) {
		t.Fatalf("unexpected order: %q", view)
	}
}

func TestBulkAssignSelectedEntries(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	_, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	delta, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Delta Labs", Code: "delta", HourlyRate: 12000, Currency: "EUR"})
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
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	app = updated.(AppModel)
	if len(app.selected) != 2 {
		t.Fatalf("selected = %d, want 2", len(app.selected))
	}
	plain := stripANSI(app.View())
	if !strings.Contains(plain, ">*") || !strings.Contains(plain, " *") {
		t.Fatalf("view missing selection markers: %q", plain)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	app = updated.(AppModel)
	if app.mode != modeAssign {
		t.Fatalf("mode = %q, want assign", app.mode)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = updated.(AppModel)
	if app.projectCursor != 1 {
		t.Fatalf("projectCursor = %d, want 1", app.projectCursor)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	confirmed := 0
	for _, entry := range entries {
		if entry.ProjectID != nil && *entry.ProjectID == delta.ID && entry.Status == "confirmed" {
			confirmed++
		}
	}
	if confirmed < 2 {
		t.Fatalf("confirmed assigned entries = %d, want at least 2", confirmed)
	}
}

func TestSingleUnassignEntry(t *testing.T) {
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
	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	for _, entry := range entries {
		if err := store.AssignEntryToProject(ctx, entry.ID, project.ID); err != nil {
			t.Fatalf("AssignEntryToProject() error = %v", err)
		}
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if !strings.Contains(stripANSI(app.View()), "Unassign") {
		t.Fatalf("assign picker missing unassign option: %q", stripANSI(app.View()))
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	refreshed, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	unassigned := 0
	for _, entry := range refreshed {
		if entry.ProjectID == nil && entry.Status == "draft" {
			unassigned++
		}
	}
	if unassigned == 0 {
		t.Fatalf("no entry unassigned: %#v", refreshed)
	}
}

func TestBulkUnassignSelectedEntries(t *testing.T) {
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
	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := store.AssignEntryToProject(ctx, entries[i].ID, project.ID); err != nil {
			t.Fatalf("AssignEntryToProject() error = %v", err)
		}
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	app = updated.(AppModel)
	if len(app.selected) != 2 {
		t.Fatalf("selected = %d, want 2", len(app.selected))
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	app = updated.(AppModel)
	if app.mode != modeAssign {
		t.Fatalf("mode = %q, want assign", app.mode)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	refreshed, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	unassigned := 0
	for _, entry := range refreshed {
		if entry.ProjectID == nil && entry.Status == "draft" {
			unassigned++
		}
	}
	if unassigned < 2 {
		t.Fatalf("unassigned entries = %d, want at least 2", unassigned)
	}
}

func TestTimelineGroupJumpKeys(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	_, _ = store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Day1", StartedAt: time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)})
	_, _ = store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Day2", StartedAt: time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)})
	_, _ = store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Day3", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)})

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	app := updated.(AppModel)
	if !strings.Contains(stripANSI(app.View()), "Day3") {
		t.Fatalf("initial view missing newest group entry: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("}")})
	app = updated.(AppModel)
	if app.cursor != 1 {
		t.Fatalf("cursor after first } = %d, want 1", app.cursor)
	}
	if !strings.Contains(stripANSI(app.View()), "Day2") {
		t.Fatalf("view missing next group entry after }: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("}")})
	app = updated.(AppModel)
	if app.cursor != 2 {
		t.Fatalf("cursor after second } = %d, want 2", app.cursor)
	}
	if !strings.Contains(stripANSI(app.View()), "Day1") {
		t.Fatalf("view missing next-next group entry after }: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("{")})
	app = updated.(AppModel)
	if app.cursor != 1 {
		t.Fatalf("cursor after { = %d, want 1", app.cursor)
	}
	if !strings.Contains(stripANSI(app.View()), "Day2") {
		t.Fatalf("view missing previous group entry after {: %q", stripANSI(app.View()))
	}
}

func TestTimelineSlashSearchAndNextPrev(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	_, _ = store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Alpha task", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)})
	_, _ = store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Beta task", StartedAt: time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)})
	_, _ = store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Beta followup", StartedAt: time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)})

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	app := updated.(AppModel)

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app = updated.(AppModel)
	if app.mode != modeSearch {
		t.Fatalf("mode = %q, want search", app.mode)
	}
	for _, r := range []rune("beta") {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	if !strings.Contains(stripANSI(app.View()), "/beta") {
		t.Fatalf("search prompt missing query: %q", stripANSI(app.View()))
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeTimeline || app.lastSearch != "beta" {
		t.Fatalf("search state not applied: mode=%q lastSearch=%q", app.mode, app.lastSearch)
	}
	if !strings.Contains(stripANSI(app.View()), "Beta task") {
		t.Fatalf("view missing first beta match: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	app = updated.(AppModel)
	if !strings.Contains(stripANSI(app.View()), "Beta followup") {
		t.Fatalf("view missing next beta match: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	app = updated.(AppModel)
	if !strings.Contains(stripANSI(app.View()), "Beta task") {
		t.Fatalf("view missing previous beta match: %q", stripANSI(app.View()))
	}
}

func TestTimelineSourceFilterCycles(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Human entry", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if err := sync.ImportClaudeFixtures(ctx, store, filepath.Join("..", "..", "testdata", "claude-sessions")); err != nil {
		t.Fatalf("ImportClaudeFixtures() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	app := updated.(AppModel)
	if app.sourceFilter != "all" {
		t.Fatalf("sourceFilter = %q, want all", app.sourceFilter)
	}
	if !strings.Contains(stripANSI(app.View()), "Human entry") || !strings.Contains(stripANSI(app.View()), "Refactor the auth module") {
		t.Fatalf("all filter missing mixed entries: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	app = updated.(AppModel)
	if app.sourceFilter != "opencode" || len(app.entries) != 0 {
		t.Fatalf("first filter step = %q len=%d", app.sourceFilter, len(app.entries))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	app = updated.(AppModel)
	if app.sourceFilter != "codex" || len(app.entries) != 0 {
		t.Fatalf("second filter step = %q len=%d", app.sourceFilter, len(app.entries))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	app = updated.(AppModel)
	if app.sourceFilter != "claude-code" || len(app.entries) != 2 {
		t.Fatalf("third filter step = %q len=%d", app.sourceFilter, len(app.entries))
	}
	if strings.Contains(stripANSI(app.View()), "Human entry") || !strings.Contains(stripANSI(app.View()), "Refactor the auth module") {
		t.Fatalf("claude filter view wrong: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	app = updated.(AppModel)
	if app.sourceFilter != "human" || len(app.entries) != 1 {
		t.Fatalf("fourth filter step = %q len=%d", app.sourceFilter, len(app.entries))
	}
	if !strings.Contains(stripANSI(app.View()), "Human entry") || strings.Contains(stripANSI(app.View()), "Refactor the auth module") {
		t.Fatalf("human filter view wrong: %q", stripANSI(app.View()))
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
