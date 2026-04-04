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
	"github.com/akoskm/hrs/internal/model"
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
		return strings.Contains(string(b), "Edit Time Entry")
	}, teatest.WithDuration(5*time.Second))
	tm.Send(tea.KeyMsg{Type: tea.KeyTab})
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
	if !strings.Contains(view, "Edit Time Entry") {
		t.Fatalf("view missing assign picker: %q", view)
	}
	if !strings.Contains(view, "Delta Labs (delta)") || !strings.Contains(view, "Elaiia (elaiia)") {
		t.Fatalf("view missing DB projects: %q", view)
	}
}

func TestAssignDialogStaysWithinScreenHeight(t *testing.T) {
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
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	app = updated.(AppModel)

	lines := strings.Split(strings.TrimRight(stripANSI(app.View()), "\n"), "\n")
	if len(lines) > 12 {
		t.Fatalf("dialog line count = %d, want <= 12", len(lines))
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Assign Project") {
		t.Fatalf("view missing dialog title: %q", strings.Join(lines, "\n"))
	}
}

func TestAssignDialogDefaultsToCurrentProject(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	entry, err := store.CreateImportedEntry(ctx, db.EntryImport{ProjectID: &project.ID, Description: "Auth", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC), Operator: "opencode", SourceRef: "sess-1", Cwd: "/Users/akoskm/Projects/hrs", Metadata: map[string]any{}})
	if err != nil {
		t.Fatalf("CreateImportedEntry() error = %v", err)
	}
	if err := store.AssignEntryToProject(ctx, entry.ID, project.ID); err != nil {
		t.Fatalf("AssignEntryToProject() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	if app.mode != modeEntryEdit || app.entryProjectCursor != 1 {
		t.Fatalf("mode/cursor = %q/%d, want entry-edit/1", app.mode, app.entryProjectCursor)
	}
}

func TestManageProjectsToggleCreateAndArchive(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "Admin",
		StartedAt:    time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	app = updated.(AppModel)
	if app.mode != modeAssign || app.dialogMode != projectDialogManage {
		t.Fatalf("dialog state = %q/%q, want assign/manage", app.mode, app.dialogMode)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	app = updated.(AppModel)
	updatedProject, err := store.ProjectByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("ProjectByID() error = %v", err)
	}
	if updatedProject.BillableDefault {
		t.Fatal("billable_default = true, want false")
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	app = updated.(AppModel)
	if app.dialogMode != projectDialogCreate {
		t.Fatalf("dialogMode = %q, want create", app.dialogMode)
	}
	for _, r := range []rune("Delta Labs") {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.dialogMode != projectDialogManage {
		t.Fatalf("dialogMode after create = %q, want manage", app.dialogMode)
	}
	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("len(projects) = %d, want 2", len(projects))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnd})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	app = updated.(AppModel)
	projects, err = store.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("len(projects) after archive = %d, want 1", len(projects))
	}
	archived, err := store.ProjectByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("ProjectByID() error = %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("archived_at = nil, want value")
	}
	if !strings.Contains(stripANSI(app.View()), "Manage Projects") {
		t.Fatalf("view missing manage dialog: %q", stripANSI(app.View()))
	}
}

func TestManageProjectsCyclesColor(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	app = updated.(AppModel)
	updatedProject, err := store.ProjectByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("ProjectByID() error = %v", err)
	}
	if updatedProject.Color == nil || project.Color == nil || *updatedProject.Color == *project.Color {
		t.Fatalf("color did not change: before=%v after=%v", project.Color, updatedProject.Color)
	}
}

func TestManageProjectsRandomizesColor(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	app = updated.(AppModel)
	updatedProject, err := store.ProjectByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("ProjectByID() error = %v", err)
	}
	if updatedProject.Color == nil || project.Color == nil || *updatedProject.Color == *project.Color {
		t.Fatalf("color did not change: before=%v after=%v", project.Color, updatedProject.Color)
	}
	if !strings.Contains(stripANSI(app.View()), "C random") {
		t.Fatalf("manage dialog missing random color help: %q", stripANSI(app.View()))
	}
}

func TestEnterOpensEntryEditDialogAndSavesDescriptionAndProject(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Delta", Code: "delta", Currency: "EUR"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	entry, err := store.CreateImportedEntry(ctx, db.EntryImport{Description: "Auth", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC), Operator: "opencode", SourceRef: "sess-edit", Cwd: "/Users/akoskm/Projects/hrs", Metadata: map[string]any{}})
	if err != nil {
		t.Fatalf("CreateImportedEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	if app.mode != modeEntryEdit {
		t.Fatalf("mode = %q, want entry-edit", app.mode)
	}
	view := stripANSI(app.View())
	if strings.Index(view, "Description") > strings.Index(view, "Project") {
		t.Fatalf("description should render before project: %q", view)
	}
	if app.entryInputField != "description" {
		t.Fatalf("entryInputField = %q, want description", app.entryInputField)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	app = updated.(AppModel)
	for _, r := range []rune("Updated") {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeTimeline {
		t.Fatalf("mode after save = %q, want timeline", app.mode)
	}
	updatedEntry, err := store.EntryByID(ctx, entry.ID)
	if err != nil {
		t.Fatalf("EntryByID() error = %v", err)
	}
	if updatedEntry.Description == nil || *updatedEntry.Description != "Auth Updated" {
		t.Fatalf("description = %v, want Auth Updated", updatedEntry.Description)
	}
	if updatedEntry.ProjectID == nil {
		t.Fatal("project_id = nil, want assigned")
	}
}

func TestDeleteClearsFocusedEntryDescription(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	entry, err := store.CreateImportedEntry(ctx, db.EntryImport{Description: "Auth", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC), Operator: "opencode", SourceRef: "sess-del", Cwd: "/Users/akoskm/Projects/hrs", Metadata: map[string]any{}})
	if err != nil {
		t.Fatalf("CreateImportedEntry() error = %v", err)
	}
	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDelete})
	app = updated.(AppModel)
	if app.entryInput != "" {
		t.Fatalf("entryInput = %q, want empty", app.entryInput)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	updatedEntry, err := store.EntryByID(ctx, entry.ID)
	if err != nil {
		t.Fatalf("EntryByID() error = %v", err)
	}
	if updatedEntry.Description == nil || *updatedEntry.Description != "" {
		t.Fatalf("description = %v, want empty", updatedEntry.Description)
	}
}

func TestPickerHighlightsFullRow(t *testing.T) {
	line := renderPickerLine("Test Project", 1, 1, newStyles(80), 20)
	if !strings.Contains(line, "Test Project") {
		t.Fatalf("line missing label: %q", line)
	}
	if lipgloss.Width(stripANSI(line)) != 20 {
		t.Fatalf("line width = %d, want 20", lipgloss.Width(stripANSI(line)))
	}
}

func TestTextWithCaret(t *testing.T) {
	if got := textWithCaret("Auth", true, true); got != "Auth_" {
		t.Fatalf("textWithCaret() = %q, want Auth_", got)
	}
}

func TestOutlinedBlockCell(t *testing.T) {
	slotStart := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)
	slotEnd := slotStart.Add(15 * time.Minute)
	itemStart := slotStart
	itemEnd := slotStart.Add(45 * time.Minute)
	if got := outlinedBlockCell(slotStart, slotEnd, itemStart, itemEnd, 12, "TUI"); !strings.Contains(got, "┌") || !strings.Contains(got, "┐") {
		t.Fatalf("start cell = %q, want outlined top", got)
	}
	midStart := slotStart.Add(15 * time.Minute)
	midEnd := midStart.Add(15 * time.Minute)
	if got := outlinedBlockCell(midStart, midEnd, itemStart, itemEnd, 12, "TUI"); !strings.Contains(got, "│") {
		t.Fatalf("mid cell = %q, want vertical borders", got)
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

func TestTimelineDayViewShowsAxisAndBlocks(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Sprint planning", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 30, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Review", StartedAt: time.Date(2026, 4, 3, 13, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 14, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	app := updated.(AppModel)

	view := stripANSI(app.View())
	if app.timelineView != timelineViewDay {
		t.Fatalf("timelineView = %q, want day", app.timelineView)
	}
	if !strings.Contains(view, "time") || !strings.Contains(view, "10:00") || !strings.Contains(view, "15:00") {
		t.Fatalf("view missing day axis: %q", view)
	}
	if !strings.Contains(view, "Friday 2026-04-03") || !strings.Contains(view, "human") || !strings.Contains(view, "gaps") || !strings.Contains(view, "Friday | focus 15:00-16:00") {
		t.Fatalf("view missing day blocks: %q", view)
	}
}

func TestTimelineDayViewSplitsOverlapsIntoLanes(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Agent A", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Agent B", StartedAt: time.Date(2026, 4, 3, 9, 30, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 30, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	app := updated.(AppModel)

	view := stripANSI(app.View())
	if !strings.Contains(view, "human 2") {
		t.Fatalf("view missing overlap lane: %q", view)
	}
}

func TestTimelineDayViewLeftRightMovesBetweenDays(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	today := dayStart(time.Now().UTC())
	dayTwo := today.AddDate(0, 0, -2)
	dayThree := today.AddDate(0, 0, -1)
	dayFour := today

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Day two", StartedAt: dayTwo.Add(9 * time.Hour), EndedAt: dayTwo.Add(10 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Day three", StartedAt: dayThree.Add(12 * time.Hour), EndedAt: dayThree.Add(13 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Day four", StartedAt: dayFour.Add(15 * time.Hour), EndedAt: dayFour.Add(16 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	app := updated.(AppModel)

	if !app.displayedDay().Equal(dayFour) {
		t.Fatalf("initial day = %s, want %s", app.displayedDay().Format("2006-01-02"), dayFour.Format("2006-01-02"))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(dayFour) {
		t.Fatalf("day after right = %s, want %s", app.displayedDay().Format("2006-01-02"), dayFour.Format("2006-01-02"))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(dayThree) {
		t.Fatalf("day after left = %s, want %s", app.displayedDay().Format("2006-01-02"), dayThree.Format("2006-01-02"))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(dayTwo) {
		t.Fatalf("day after second left = %s, want %s", app.displayedDay().Format("2006-01-02"), dayTwo.Format("2006-01-02"))
	}

	if got := descriptionOrID(app.entries[app.cursor]); got != "Day two" {
		t.Fatalf("focus on returned day = %q, want Day two", got)
	}

	if !strings.Contains(stripANSI(app.View()), "left/right day") {
		t.Fatalf("day status missing nav hint: %q", stripANSI(app.View()))
	}
}

func TestTimelineDayViewTJumpsToToday(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	today := dayStart(time.Now().UTC())
	otherDay := today.AddDate(0, 0, -2)

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Old", StartedAt: otherDay.Add(9 * time.Hour), EndedAt: otherDay.Add(10 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	app := updated.(AppModel)
	if !app.displayedDay().Equal(otherDay) {
		t.Fatalf("initial day = %s, want %s", app.displayedDay().Format("2006-01-02"), otherDay.Format("2006-01-02"))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(today) {
		t.Fatalf("day after t = %s, want %s", app.displayedDay().Format("2006-01-02"), today.Format("2006-01-02"))
	}
	if app.dayFocusKind != "slot" {
		t.Fatalf("focus kind after t = %q, want slot", app.dayFocusKind)
	}
}

func TestDayViewUpDownMovesSlot(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.InitializeTodayTimelineView()
	model.daySlotSpan = 15 * time.Minute
	model.daySlotStart = maxSlotStartForDay(model.displayedDay(), model.daySlotSpan).Add(-time.Hour)
	start := model.daySlotStart
	model.moveSlot(15*time.Minute, 15*time.Minute)
	if model.daySlotStart.Sub(start) != 15*time.Minute {
		t.Fatalf("slot delta after down = %s, want 15m", model.daySlotStart.Sub(start))
	}
	model.moveSlot(-time.Hour, time.Hour)
	if model.daySlotSpan != time.Hour {
		t.Fatalf("slot span after shift+up = %s, want 1h", model.daySlotSpan)
	}
}

func TestDayViewPastTodayClampsAtEndOfDay(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.InitializeTodayTimelineView()
	today := dayStart(time.Now())
	model.dayDate = today
	model.daySlotSpan = 15 * time.Minute
	model.daySlotStart = maxSlotStartForDay(today, model.daySlotSpan)
	model.moveSlot(15*time.Minute, 15*time.Minute)
	if !model.displayedDay().Equal(today) {
		t.Fatalf("displayedDay = %s, want today", model.displayedDay().Format("2006-01-02"))
	}
	if !model.daySlotStart.Equal(maxSlotStartForDay(today, model.daySlotSpan)) {
		t.Fatalf("daySlotStart = %s, want clamped end of day", model.daySlotStart.Format(time.RFC3339))
	}
}

func TestGapDialogDefaultsToSelectedSlotRange(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.InitializeTodayTimelineView()
	model.daySlotStart = time.Date(2026, 4, 3, 15, 15, 0, 0, time.Local)
	model.daySlotSpan = time.Hour
	model.dayFocusKind = "slot"
	rangeSel := model.selectedCreateRange()
	if rangeSel == nil || formatRange(rangeSel.start, &rangeSel.end) != "15:15-16:15" {
		t.Fatalf("selected range = %#v, want 15:15-16:15", rangeSel)
	}
	model.openGapEntryDialog()
	if model.gapInputField != "description" {
		t.Fatalf("gapInputField = %q, want description", model.gapInputField)
	}
	view := stripANSI(renderGapEntryDialog(model, newStyles(100), ""))
	if strings.Index(view, "Description") > strings.Index(view, "Project") {
		t.Fatalf("description should render before project: %q", view)
	}
}

func TestTimelineDayViewDoesNotMovePastToday(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	today := dayStart(time.Now().UTC())

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Today", StartedAt: today.Add(9 * time.Hour), EndedAt: today.Add(10 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(today) {
		t.Fatalf("day after right on today = %s, want %s", app.displayedDay().Format("2006-01-02"), today.Format("2006-01-02"))
	}
}

func TestTimelineDayViewCreateManualEntryFromGap(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Morning", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Noon", StartedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 13, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	app = updated.(AppModel)
	if app.dayFocusKind != "gap" {
		t.Fatalf("focus kind = %q, want gap", app.dayFocusKind)
	}
	if gap := app.focusedGap(); gap == nil || formatRange(gap.start, &gap.end) != "12:00-14:00" {
		t.Fatalf("focused gap = %#v, want 12:00-14:00", app.focusedGap())
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	app = updated.(AppModel)
	if app.mode != modeGapEntry {
		t.Fatalf("mode = %q, want gap-entry", app.mode)
	}
	for _, r := range []rune("Deep work") {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeTimeline {
		t.Fatalf("mode after create = %q, want timeline", app.mode)
	}
	if got := descriptionOrID(app.entries[app.cursor]); got != "Deep work" {
		t.Fatalf("focus after create = %q, want Deep work", got)
	}
	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	found := false
	for _, entry := range entries {
		if entry.Description != nil && *entry.Description == "Deep work" {
			if formatRange(entry.StartedAt, entry.EndedAt) != "12:00-14:00" {
				t.Fatalf("created range = %s, want 12:00-14:00", formatRange(entry.StartedAt, entry.EndedAt))
			}
			found = true
		}
	}
	if !found {
		t.Fatal("manual gap entry not created")
	}
}

func TestRecentAgentSessionRendersAsActiveUntilNow(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Minute)
	start := now.Add(-30 * time.Minute)
	end := now.Add(-5 * time.Minute)
	if _, err := store.CreateImportedEntry(ctx, db.EntryImport{
		Description: "Active opencode session",
		StartedAt:   start,
		EndedAt:     end,
		Operator:    "opencode",
		SourceRef:   "live-session",
		Cwd:         "/Users/akoskm/Projects/hrs",
		Metadata:    map[string]any{},
	}); err != nil {
		t.Fatalf("CreateImportedEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	entry := model.entries[model.cursor]
	blockEnd := timelineBlockEnd(entry)
	if blockEnd.Before(now.Add(-2 * time.Minute)) {
		t.Fatalf("timelineBlockEnd() = %s, want near now %s", blockEnd.Format(time.RFC3339), now.Format(time.RFC3339))
	}
	view := stripANSI(model.View())
	if !strings.Contains(view, "opencode") {
		t.Fatalf("view missing opencode lane: %q", view)
	}
}

func TestSessionInspectorLoadsSourceDetail(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	if err := sync.ImportClaudeFixtures(ctx, store, filepath.Join("..", "..", "testdata", "claude-sessions")); err != nil {
		t.Fatalf("ImportClaudeFixtures() error = %v", err)
	}
	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	model.inspectorTab = inspectorSession
	view := stripANSI(model.View())
	if !strings.Contains(view, "Source: claude-code") || !strings.Contains(view, "Messages:") || !strings.Contains(view, "Path:") {
		t.Fatalf("session inspector missing source detail: %q", view)
	}
}

func TestTodayGapsDoNotExtendIntoFuture(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Minute)
	start := now.Add(-2 * time.Hour)
	end := now.Add(-30 * time.Minute)
	if _, err := store.CreateImportedEntry(ctx, db.EntryImport{
		Description: "Ended agent session",
		StartedAt:   start,
		EndedAt:     end,
		Operator:    "opencode",
		SourceRef:   "ended-session",
		Cwd:         "/Users/akoskm/Projects/hrs",
		Metadata:    map[string]any{},
	}); err != nil {
		t.Fatalf("CreateImportedEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	gaps := dayGapsForIndices(model.entries, model.dayEntryIndices(model.displayedDay().Format("2006-01-02")), model.displayedDay())
	if len(gaps) == 0 {
		t.Fatal("len(gaps) = 0, want at least one")
	}
	latest := gaps[len(gaps)-1]
	if latest.end.After(time.Now().In(time.Local).Add(time.Minute)) {
		t.Fatalf("latest gap end = %s, want no later than now", latest.end.Format(time.RFC3339))
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
		t.Fatalf("edit dialog missing unassign option: %q", stripANSI(app.View()))
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyUp})
	app = updated.(AppModel)
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
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 20})
	app := updated.(AppModel)
	if app.sourceFilter != "all" {
		t.Fatalf("sourceFilter = %q, want all", app.sourceFilter)
	}
	if !strings.Contains(stripANSI(app.View()), "P projects") {
		t.Fatalf("status bar missing project manage hint: %q", stripANSI(app.View()))
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

func TestSyncStatusBarAnimatesWhileSyncing(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	model, err := NewAppModelWithSync(ctx, store, func() error { return nil })
	if err != nil {
		t.Fatalf("NewAppModelWithSync() error = %v", err)
	}
	model.width = 120
	model.height = 20
	model.syncing = true
	model.syncFrame = 2
	first := renderStatusBar(model, 120)
	firstSpinner := syncSpinnerFrame(model.syncFrame)
	model.syncFrame = 3
	secondSpinner := syncSpinnerFrame(model.syncFrame)
	if !strings.Contains(first, "Syncing") {
		t.Fatalf("first status missing sync bar: %q", first)
	}
	if firstSpinner == secondSpinner {
		t.Fatalf("sync bar did not animate: %q", first)
	}
	model.syncing = false
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	if !app.syncing {
		t.Fatal("syncing = false after r, want true")
	}
	if cmd == nil {
		t.Fatal("cmd = nil after r, want sync commands")
	}
	updated, _ = model.Update(syncDoneMsg{})
	app = updated.(AppModel)
	if app.syncing {
		t.Fatal("syncing = true, want false")
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

func descriptionOrID(entry model.TimeEntryDetail) string {
	if entry.Description != nil && *entry.Description != "" {
		return *entry.Description
	}
	return entry.ID
}
