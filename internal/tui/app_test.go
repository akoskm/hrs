package tui

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"
)

func TestTimelineRendersAndAssigns(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "Auth refactor",
		StartedAt:    time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}

	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(120, 30))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		out := stripANSI(string(b))
		return strings.Contains(out, "Auth refactor") && strings.Contains(out, "confirmed")
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
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "Work",
		StartedAt:    time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
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
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "Work",
		StartedAt:    time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
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
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "Work",
		StartedAt:    time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
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

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "Auth",
		StartedAt:    time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
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
	entry, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "Auth",
		StartedAt:    time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
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

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	entry, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "p",
		Description:  "Auth",
		StartedAt:    time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
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

func TestEditDialogUpdatesTimes(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	entry, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "p",
		Description:  "Auth",
		StartedAt:    time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDelete})
	app = updated.(AppModel)
	for _, r := range []rune("09:30") {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDelete})
	app = updated.(AppModel)
	for _, r := range []rune("10:45") {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	updatedEntry, err := store.EntryByID(ctx, entry.ID)
	if err != nil {
		t.Fatalf("EntryByID() error = %v", err)
	}
	if formatRange(updatedEntry.StartedAt, updatedEntry.EndedAt) != "09:30-10:45" {
		t.Fatalf("updated range = %s, want 09:30-10:45", formatRange(updatedEntry.StartedAt, updatedEntry.EndedAt))
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
	if got := textWithCaret("Auth", true, true); got != "Auth█" {
		t.Fatalf("textWithCaret() = %q, want Auth█", got)
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
	if !strings.Contains(view, "Friday 2026-04-03") || !strings.Contains(view, "activity") || !strings.Contains(view, "Friday | focus") {
		t.Fatalf("view missing day blocks: %q", view)
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

func TestGapDialogUsesEditedTimesOnCreate(t *testing.T) {
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
	model.openGapEntryDialog()
	model.gapInput = "Deep work"
	model.gapProjectCursor = 1
	model.gapInputField = "start"
	model.gapStartInput = "15:30"
	model.gapInputField = "end"
	model.gapEndInput = "16:45"
	model.createGapEntry()
	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if formatRange(entries[0].StartedAt, entries[0].EndedAt) != "15:30-16:45" {
		t.Fatalf("created range = %s, want 15:30-16:45", formatRange(entries[0].StartedAt, entries[0].EndedAt))
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
	if app.dayFocusKind != "slot" {
		t.Fatalf("focus kind = %q, want slot", app.dayFocusKind)
	}
	rng := app.selectedCreateRange()
	if rng == nil || formatRange(rng.start, &rng.end) != "12:00-14:00" {
		t.Fatalf("selected range = %#v, want 12:00-14:00", rng)
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
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = updated.(AppModel)
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

func TestTodayGapsDoNotExtendIntoFuture(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	start := now.Add(-2 * time.Hour)
	end := now.Add(-30 * time.Minute)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "p",
		Description:  "Ended session",
		StartedAt:    start,
		EndedAt:      end,
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
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
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Entry A", StartedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Entry B", StartedAt: time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
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

	_, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Work A", StartedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
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

	_, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Work A", StartedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Work B", StartedAt: time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
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

func TestCycleInspectorTab(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	if app.inspectorTab != inspectorOverview {
		t.Fatalf("initial tab = %q, want overview", app.inspectorTab)
	}
	// tab forward: overview -> actions -> overview
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	if app.inspectorTab != inspectorActions {
		t.Fatalf("after tab = %q, want actions", app.inspectorTab)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	if app.inspectorTab != inspectorOverview {
		t.Fatalf("after 2nd tab = %q, want overview (wrap)", app.inspectorTab)
	}
	// shift+tab backward: overview -> actions
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	app = updated.(AppModel)
	if app.inspectorTab != inspectorActions {
		t.Fatalf("after shift+tab = %q, want actions", app.inspectorTab)
	}
}

func TestBackspaceInEntryEditDialog(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Hello", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// Open edit dialog
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeEntryEdit {
		t.Fatalf("mode = %q, want entry-edit", app.mode)
	}
	if app.entryInput != "Hello" {
		t.Fatalf("entryInput = %q, want Hello", app.entryInput)
	}
	// Backspace removes last char
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	app = updated.(AppModel)
	if app.entryInput != "Hell" {
		t.Fatalf("after backspace entryInput = %q, want Hell", app.entryInput)
	}
	// Tab to start field, type, backspace
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	// Should be on "project" now, tab again to "start"
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	if app.entryInputField != "start" {
		t.Fatalf("field = %q, want start", app.entryInputField)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	app = updated.(AppModel)
	// After backspace, last char removed
	expectedStart := app.entryStartInput
	if len(expectedStart) == 0 {
		t.Fatal("start input empty before backspace")
	}
	// backspace already applied above, verify it shortened
	origStart := clock(time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC))
	if app.entryStartInput != origStart[:len(origStart)-1] {
		t.Fatalf("start after backspace = %q, want %q", app.entryStartInput, origStart[:len(origStart)-1])
	}
	// Tab to end field
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	if app.entryInputField != "end" {
		t.Fatalf("field = %q, want end", app.entryInputField)
	}
	origEnd := app.entryEndInput
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	app = updated.(AppModel)
	if app.entryEndInput != origEnd[:len(origEnd)-1] {
		t.Fatalf("end after backspace = %q, want %q", app.entryEndInput, origEnd[:len(origEnd)-1])
	}
}

func TestBackspaceAndClearInGapDialog(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	today := dayStart(time.Now())
	app.timelineView = timelineViewDay
	app.dayDate = today
	app.daySlotSpan = 15 * time.Minute
	app.daySlotStart = time.Date(today.Year(), today.Month(), today.Day(), 9, 0, 0, 0, time.Local)
	app.dayFocusKind = "slot"

	// Open gap dialog
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeGapEntry {
		t.Fatalf("mode = %q, want gap-entry", app.mode)
	}
	// Type then backspace
	for _, r := range "abc" {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	if app.gapInput != "abc" {
		t.Fatalf("gapInput = %q, want abc", app.gapInput)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	app = updated.(AppModel)
	if app.gapInput != "ab" {
		t.Fatalf("after backspace gapInput = %q, want ab", app.gapInput)
	}
	// Delete clears field
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDelete})
	app = updated.(AppModel)
	if app.gapInput != "" {
		t.Fatalf("after delete gapInput = %q, want empty", app.gapInput)
	}
	// Tab to start, backspace
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	// Tab cycles: description -> project -> start
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	if app.gapInputField != "start" {
		t.Fatalf("field = %q, want start", app.gapInputField)
	}
	orig := app.gapStartInput
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	app = updated.(AppModel)
	if len(app.gapStartInput) != len(orig)-1 {
		t.Fatalf("start after backspace = %q, want len %d", app.gapStartInput, len(orig)-1)
	}
	// Delete clears start
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDelete})
	app = updated.(AppModel)
	if app.gapStartInput != "" {
		t.Fatalf("start after delete = %q, want empty", app.gapStartInput)
	}
	// Tab to end, backspace
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	if app.gapInputField != "end" {
		t.Fatalf("field = %q, want end", app.gapInputField)
	}
	origEnd := app.gapEndInput
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	app = updated.(AppModel)
	if len(app.gapEndInput) != len(origEnd)-1 {
		t.Fatalf("end after backspace = %q, want len %d", app.gapEndInput, len(origEnd)-1)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDelete})
	app = updated.(AppModel)
	if app.gapEndInput != "" {
		t.Fatalf("end after delete = %q, want empty", app.gapEndInput)
	}
}

func TestActionInspectorLines(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	// Slot mode
	app.dayFocusKind = "slot"
	lines := actionInspectorLines(app)
	if len(lines) == 0 || !strings.Contains(lines[0], "create entry") {
		t.Fatalf("slot action lines = %v, want create entry hint", lines)
	}

	// Gap mode
	app.dayFocusKind = "gap"
	lines = actionInspectorLines(app)
	if len(lines) == 0 || !strings.Contains(lines[0], "manual entry") {
		t.Fatalf("gap action lines = %v, want manual entry hint", lines)
	}

	// Entry mode
	app.dayFocusKind = "entry"
	lines = actionInspectorLines(app)
	if len(lines) == 0 || !strings.Contains(lines[0], "edit") {
		t.Fatalf("entry action lines = %v, want edit hint", lines)
	}

	// With selections
	app.selected["some-id"] = true
	lines = actionInspectorLines(app)
	hasAssign := false
	for _, l := range lines {
		if strings.Contains(l, "assign selected") {
			hasAssign = true
		}
	}
	if !hasAssign {
		t.Fatalf("entry action lines with selection = %v, want assign hint", lines)
	}
}

func TestDeleteConfirmDialogRenders(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "To remove", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app = updated.(AppModel)

	// Press d to open delete dialog
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = updated.(AppModel)
	if app.mode != modeDeleteConfirm {
		t.Fatalf("mode = %q, want delete-confirm", app.mode)
	}

	view := stripANSI(app.View())
	if !strings.Contains(view, "Delete Entry") {
		t.Fatalf("view missing 'Delete Entry' title")
	}
	if !strings.Contains(view, "To remove") {
		t.Fatalf("view missing entry description")
	}
	if !strings.Contains(view, "y/n") {
		t.Fatalf("view missing y/n prompt")
	}
}

func TestJumpToNow(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	// Create entry around current time
	now := time.Now().UTC()
	start := now.Add(-30 * time.Minute)
	end := now.Add(30 * time.Minute)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Current", StartedAt: start, EndedAt: end}); err != nil {
		t.Fatalf("err = %v", err)
	}
	// Create entry far in the past
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Old", StartedAt: time.Date(2026, 4, 5, 2, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 5, 3, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	app.SetDefaultTimelineView("day")
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	app = updated.(AppModel)

	// Move to old entry
	app.cursor = 0
	app.dayFocusKind = "entry"

	// Press n to jump to now — should land on "Current" entry, not a gap
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	app = updated.(AppModel)

	if app.dayFocusKind != "entry" {
		t.Fatalf("after n, focus = %q, want entry (not gap/slot)", app.dayFocusKind)
	}
	idx := app.effectiveEntryIndex()
	if idx < 0 || idx >= len(app.entries) {
		t.Fatalf("after n, no entry focused (idx=%d)", idx)
	}
	desc := descriptionOrID(app.entries[idx])
	if desc != "Current" {
		t.Fatalf("after n, focused = %q, want Current", desc)
	}
}

func TestRestoreStateAfterReload(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "First", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Second", StartedAt: time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}

	app, err := NewAppModelWithSync(ctx, store, func() error { return nil })
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// Move cursor to second entry
	app.cursor = 1

	// Simulate sync completing
	updated, _ := app.Update(syncDoneMsg{err: nil})
	app = updated.(AppModel)

	// After reload, should still have entries and cursor should be valid
	if len(app.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(app.entries))
	}
	if app.cursor < 0 || app.cursor >= len(app.entries) {
		t.Fatalf("cursor = %d out of range", app.cursor)
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

func TestDayViewJKNeverStaysOnSameEntry(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	// Three entries with gaps between them:
	// A: 09:00-10:00, gap 10:00-11:00, B: 11:00-12:00, gap 12:00-13:00, C: 13:00-14:00
	for _, e := range []struct{ desc, start, end string }{
		{"A", "09:00", "10:00"},
		{"B", "11:00", "12:00"},
		{"C", "13:00", "14:00"},
	} {
		s, _ := time.Parse("2006-01-02 15:04", "2026-04-03 "+e.start)
		end, _ := time.Parse("2006-01-02 15:04", "2026-04-03 "+e.end)
		if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: e.desc, StartedAt: s, EndedAt: end}); err != nil {
			t.Fatalf("CreateManualEntry(%s) error = %v", e.desc, err)
		}
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	app.SetDefaultTimelineView("day")
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	app = updated.(AppModel)

	// Start on entry C (last entry, default focus)
	if app.dayFocusKind != "entry" {
		t.Fatalf("initial focus = %q, want entry", app.dayFocusKind)
	}
	startDesc := descriptionOrID(app.entries[app.cursor])
	if startDesc != "C" {
		t.Fatalf("initial entry = %q, want C", startDesc)
	}

	// Navigate backward with k — should never stay on same entry
	seen := []string{"C"}
	for i := 0; i < 6; i++ {
		prevIdx := app.effectiveEntryIndex()
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		app = updated.(AppModel)
		currIdx := app.effectiveEntryIndex()
		if currIdx >= 0 && currIdx == prevIdx {
			t.Fatalf("k press %d stayed on same entry index %d", i+1, currIdx)
		}
		if currIdx >= 0 {
			seen = append(seen, descriptionOrID(app.entries[currIdx]))
		} else {
			seen = append(seen, "slot")
		}
	}

	// Verify we visited gap and entry alternately (C -> gap -> B -> gap -> A -> slot)
	hasB := false
	hasA := false
	for _, s := range seen {
		if s == "B" {
			hasB = true
		}
		if s == "A" {
			hasA = true
		}
	}
	if !hasA || !hasB {
		t.Fatalf("didn't visit all entries, seen = %v", seen)
	}

	// Navigate forward with j — should never stay on same entry
	for i := 0; i < 6; i++ {
		prevIdx := app.effectiveEntryIndex()
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		app = updated.(AppModel)
		currIdx := app.effectiveEntryIndex()
		if currIdx >= 0 && currIdx == prevIdx {
			t.Fatalf("j press %d stayed on same entry index %d", i+1, currIdx)
		}
	}
}

func TestSpaceMarkSlotRangeAndCreateEntry(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	// Start in day view at a known slot
	now := time.Now()
	today := dayStart(now)
	app.timelineView = timelineViewDay
	app.dayDate = today
	app.daySlotSpan = 15 * time.Minute
	app.daySlotStart = time.Date(today.Year(), today.Month(), today.Day(), 9, 0, 0, 0, time.Local)
	app.dayFocusKind = "slot"
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	app = updated.(AppModel)

	// Verify we're on a slot
	if app.dayFocusKind != "slot" {
		t.Fatalf("initial focus = %q, want slot", app.dayFocusKind)
	}
	if !app.slotMarkStart.IsZero() {
		t.Fatal("slotMarkStart should be zero before space")
	}

	// Press space to start marking
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	app = updated.(AppModel)
	if app.slotMarkStart.IsZero() {
		t.Fatal("slotMarkStart should be set after space")
	}
	markStart := app.slotMarkStart

	// Move down 4 times (4 * 15min = 1 hour)
	for i := 0; i < 4; i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
		app = updated.(AppModel)
	}

	// Verify mark is still anchored and range expanded
	if app.slotMarkStart != markStart {
		t.Fatalf("mark anchor moved: got %v, want %v", app.slotMarkStart, markStart)
	}
	rng := app.selectedCreateRange()
	if rng == nil {
		t.Fatal("selectedCreateRange() is nil")
	}
	// Should span from 09:00 to 10:15 (anchor 09:00-09:15, current 10:00-10:15)
	expectedStart := "09:00"
	expectedEnd := "10:15"
	gotRange := formatRange(rng.start, &rng.end)
	if gotRange != expectedStart+"-"+expectedEnd {
		t.Fatalf("marked range = %s, want %s-%s", gotRange, expectedStart, expectedEnd)
	}

	// Press enter to open gap dialog — should have the marked range
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeGapEntry {
		t.Fatalf("mode = %q, want gap-entry", app.mode)
	}
	if app.gapStartInput != "09:00" {
		t.Fatalf("gapStartInput = %q, want 09:00", app.gapStartInput)
	}
	if app.gapEndInput != "10:15" {
		t.Fatalf("gapEndInput = %q, want 10:15", app.gapEndInput)
	}

	// Mark should be cleared after opening dialog
	if !app.slotMarkStart.IsZero() {
		t.Fatal("slotMarkStart should be cleared after opening dialog")
	}

	// Type description, select project, save
	for _, r := range "Marked entry" {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)

	if app.mode != modeTimeline {
		t.Fatalf("mode after save = %q, want timeline", app.mode)
	}

	// Verify entry was created with correct range
	entries, _ := store.ListEntries(ctx)
	found := false
	for _, e := range entries {
		if e.Description != nil && *e.Description == "Marked entry" {
			r := formatRange(e.StartedAt, e.EndedAt)
			if r != "09:00-10:15" {
				t.Fatalf("created entry range = %s, want 09:00-10:15", r)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("marked entry not found in DB")
	}

	// --- Now edit the created entry ---
	// Cursor should be on the new entry after create
	if app.entries[app.cursor].Description == nil || *app.entries[app.cursor].Description != "Marked entry" {
		t.Fatalf("cursor not on created entry, got %q", descriptionOrID(app.entries[app.cursor]))
	}

	// Press enter to open edit dialog
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeEntryEdit {
		t.Fatalf("mode = %q, want entry-edit", app.mode)
	}
	if app.entryInput != "Marked entry" {
		t.Fatalf("entryInput = %q, want 'Marked entry'", app.entryInput)
	}

	// Clear description and type new one
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDelete})
	app = updated.(AppModel)
	if app.entryInput != "" {
		t.Fatalf("entryInput after delete = %q, want empty", app.entryInput)
	}
	for _, r := range "Updated description" {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}

	// Press enter to save
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeTimeline {
		t.Fatalf("mode after edit save = %q, want timeline", app.mode)
	}

	// Verify description updated in DB
	entries, _ = store.ListEntries(ctx)
	foundUpdated := false
	for _, e := range entries {
		if e.Description != nil && *e.Description == "Updated description" {
			r := formatRange(e.StartedAt, e.EndedAt)
			if r != "09:00-10:15" {
				t.Fatalf("edited entry range changed: %s, want 09:00-10:15", r)
			}
			foundUpdated = true
		}
	}
	if !foundUpdated {
		t.Fatal("edited entry not found in DB")
	}

	// Verify timeline reflects updated description
	if app.entries[app.cursor].Description == nil || *app.entries[app.cursor].Description != "Updated description" {
		t.Fatalf("timeline not updated, got %q", descriptionOrID(app.entries[app.cursor]))
	}
}

func TestShiftUpDownSelectsSingleHourSlot(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	today := dayStart(time.Now())
	app.timelineView = timelineViewDay
	app.dayDate = today
	app.daySlotSpan = 15 * time.Minute
	app.daySlotStart = time.Date(today.Year(), today.Month(), today.Day(), 9, 0, 0, 0, time.Local)
	app.dayFocusKind = "slot"
	app.slotMarkStart = time.Time{}
	app.slotMarkSpan = 0

	// shift+down: should select a single 1-hour slot at 10:00-11:00
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	app = updated.(AppModel)

	rng := app.selectedCreateRange()
	if rng == nil {
		t.Fatal("selectedCreateRange() is nil after shift+down")
	}
	if clock(rng.start) != "10:00" {
		t.Fatalf("range start = %s, want 10:00", clock(rng.start))
	}
	if clock(rng.end) != "11:00" {
		t.Fatalf("range end = %s, want 11:00", clock(rng.end))
	}

	// Another shift+down: should move to 11:00-12:00, NOT accumulate
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	app = updated.(AppModel)
	rng = app.selectedCreateRange()
	if rng == nil {
		t.Fatal("selectedCreateRange() is nil after 2nd shift+down")
	}
	if clock(rng.start) != "11:00" {
		t.Fatalf("range start after 2nd shift+down = %s, want 11:00", clock(rng.start))
	}
	if clock(rng.end) != "12:00" {
		t.Fatalf("range end after 2nd shift+down = %s, want 12:00", clock(rng.end))
	}

	// shift+up: should move to 10:00-11:00
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	app = updated.(AppModel)
	rng = app.selectedCreateRange()
	if rng == nil {
		t.Fatal("selectedCreateRange() is nil after shift+up")
	}
	if clock(rng.start) != "10:00" {
		t.Fatalf("range start after shift+up = %s, want 10:00", clock(rng.start))
	}
	if clock(rng.end) != "11:00" {
		t.Fatalf("range end after shift+up = %s, want 11:00", clock(rng.end))
	}

	// esc should clear the selection — no mark, back to 15min slot
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app = updated.(AppModel)
	if !app.slotMarkStart.IsZero() {
		t.Fatal("slotMarkStart not cleared by esc")
	}
	if app.slotMarkSpan != 0 {
		t.Fatalf("slotMarkSpan = %v, want 0", app.slotMarkSpan)
	}
	rng = app.selectedCreateRange()
	if rng == nil {
		t.Fatal("selectedCreateRange() nil after esc")
	}
	// After esc the slot should be back to 15min
	dur := rng.end.Sub(rng.start)
	if dur != 15*time.Minute {
		t.Fatalf("range duration after esc = %v, want 15m", dur)
	}
	// The range should just be the current daySlotStart, not the marked one
	if rng.start != app.daySlotStart {
		t.Fatalf("range start after esc = %s, want current slot %s", clock(rng.start), clock(app.daySlotStart))
	}
}

func TestSpaceTogglesCancelsMark(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	today := dayStart(time.Now())
	app.timelineView = timelineViewDay
	app.dayDate = today
	app.daySlotSpan = 15 * time.Minute
	app.daySlotStart = time.Date(today.Year(), today.Month(), today.Day(), 9, 0, 0, 0, time.Local)
	app.dayFocusKind = "slot"

	// Space starts mark
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	app = updated.(AppModel)
	if app.slotMarkStart.IsZero() {
		t.Fatal("mark not started")
	}

	// Space again cancels mark
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	app = updated.(AppModel)
	if !app.slotMarkStart.IsZero() {
		t.Fatal("mark not cancelled by second space")
	}

	// Esc also cancels mark
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app = updated.(AppModel)
	if !app.slotMarkStart.IsZero() {
		t.Fatal("mark not cancelled by esc")
	}
}

func TestDayViewJKRenderedOutputChangesEachPress(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	// Two entries with a gap, and one in the afternoon
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "manual work", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 30, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "follow up", StartedAt: time.Date(2026, 4, 3, 10, 30, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "afternoon", StartedAt: time.Date(2026, 4, 3, 14, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 15, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	app.SetDefaultTimelineView("day")
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	app = updated.(AppModel)

	// Navigate forward with j, verify effective entry changes each press
	// (until we pass the last entry and hit a boundary slot)
	for i := 0; i < 8; i++ {
		prevIdx := app.effectiveEntryIndex()
		prevFocus := app.dayFocusKind
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		app = updated.(AppModel)
		currIdx := app.effectiveEntryIndex()
		// If we were on an entry, we must move to a different one or to a slot
		if prevFocus == "entry" || (prevFocus == "slot" && prevIdx >= 0) {
			if currIdx >= 0 && currIdx == prevIdx {
				t.Fatalf("j press %d stayed on same entry index %d (focus=%s)", i+1, currIdx, app.dayFocusKind)
			}
		}
	}
}

func TestDayViewJKNavigatesAndEnterEdits(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.UTC)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Morning work", StartedAt: today, EndedAt: today.Add(time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	app.InitializeTodayTimelineView()
	// position slot before the entry so j (forward) finds it
	app.daySlotStart = today.Add(-time.Hour).In(time.Local)
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	app = updated.(AppModel)

	if app.dayFocusKind != "slot" {
		t.Fatalf("initial focus = %q, want slot", app.dayFocusKind)
	}

	// j should jump to the entry
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app = updated.(AppModel)
	if app.dayFocusKind != "entry" {
		t.Fatalf("after j focus = %q, want entry", app.dayFocusKind)
	}

	// enter should open edit dialog
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeEntryEdit {
		t.Fatalf("after enter mode = %q, want entry-edit", app.mode)
	}

	// enter should save
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeTimeline {
		t.Fatalf("after save mode = %q, want timeline", app.mode)
	}

	// j past last entry should go to slot
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app = updated.(AppModel)
	// could be entry (re-focused) or gap or slot depending on items
	// press j again to go past
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app = updated.(AppModel)
	if app.dayFocusKind != "slot" {
		t.Fatalf("after navigating past entries focus = %q, want slot", app.dayFocusKind)
	}
}

func TestDeleteEntryConfirmDialog(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	entry, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "p",
		Description:  "To delete",
		StartedAt:    time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}

	// press d to open confirm dialog
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = updated.(AppModel)
	if app.mode != modeDeleteConfirm {
		t.Fatalf("mode = %q, want %q", app.mode, modeDeleteConfirm)
	}
	if app.confirmDeleteID != entry.ID {
		t.Fatalf("confirmDeleteID = %q, want %q", app.confirmDeleteID, entry.ID)
	}

	// press n to cancel
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	app = updated.(AppModel)
	if app.mode != modeTimeline {
		t.Fatalf("mode after cancel = %q, want %q", app.mode, modeTimeline)
	}
	entries, _ := store.ListEntries(ctx)
	if len(entries) != 1 {
		t.Fatalf("entry count after cancel = %d, want 1", len(entries))
	}

	// press d then y to confirm delete
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	app = updated.(AppModel)
	if app.mode != modeTimeline {
		t.Fatalf("mode after confirm = %q, want %q", app.mode, modeTimeline)
	}
	entries, _ = store.ListEntries(ctx)
	if len(entries) != 0 {
		t.Fatalf("entry count after delete = %d, want 0", len(entries))
	}
}

func descriptionOrID(entry model.TimeEntryDetail) string {
	if entry.Description != nil && *entry.Description != "" {
		return *entry.Description
	}
	return entry.ID
}

func TestTimelineBlockLabelIncludesBranchForAgentEntries(t *testing.T) {
	desc := "Fix auth"
	branch := "feat/auth"
	agent := model.TimeEntryDetail{TimeEntry: model.TimeEntry{Description: &desc, GitBranch: &branch, Operator: "claude-code"}}
	got := timelineBlockLabel(agent)
	if got != "Fix auth [feat/auth]" {
		t.Fatalf("agent label = %q, want %q", got, "Fix auth [feat/auth]")
	}

	human := model.TimeEntryDetail{TimeEntry: model.TimeEntry{Description: &desc, GitBranch: &branch, Operator: "human"}}
	got = timelineBlockLabel(human)
	if got != "Fix auth" {
		t.Fatalf("human label = %q, want %q", got, "Fix auth")
	}
}

func TestAutoSyncOnInit(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	m, err := NewAppModelWithSync(ctx, store, func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("NewAppModelWithSync() error = %v", err)
	}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil cmd, expected sync batch")
	}

	// without syncFn, Init should return nil
	m2, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	if m2.Init() != nil {
		t.Fatal("Init() without syncFn should return nil")
	}
}

func TestDayViewRendersTimelineWithNoEntries(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.InitializeTodayTimelineView()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app := updated.(AppModel)
	view := stripANSI(app.View())

	// timeline must render even with zero entries
	if strings.Contains(view, "no entries") {
		t.Fatal("day view should not show 'no entries' — it should render the timeline grid")
	}
	if !strings.Contains(view, "time") || !strings.Contains(view, "activity") {
		t.Fatalf("day view missing column headers, got:\n%s", view)
	}
	// must have time labels
	if !strings.Contains(view, ":00") {
		t.Fatalf("day view missing time labels, got:\n%s", view)
	}
	// must have status bar
	if !strings.Contains(view, "entries") && !strings.Contains(view, "day") {
		t.Fatalf("day view missing status bar, got:\n%s", view)
	}
}

func TestDayViewRendersEntriesAndActivity(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Sprint planning", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	// add activity slots
	if err := store.UpsertActivitySlots(ctx, []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 3, 8, 0, 0, 0, time.UTC), Operator: "claude-code", MsgCount: 5, FirstText: "fix auth module", Cwd: "/tmp"},
		{SlotTime: time.Date(2026, 4, 3, 8, 15, 0, 0, time.UTC), Operator: "claude-code", MsgCount: 3, FirstText: "add test coverage", Cwd: "/tmp"},
	}); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app := updated.(AppModel)
	view := stripANSI(app.View())

	// must show time column and activity column headers
	if !strings.Contains(view, "time") || !strings.Contains(view, "activity") {
		t.Fatalf("missing column headers, got:\n%s", view)
	}
	// must show the manual entry
	if !strings.Contains(view, "Sprint planning") {
		t.Fatalf("missing entry text in view, got:\n%s", view)
	}
	// must show activity marker text
	if !strings.Contains(view, "fix auth module") {
		t.Fatalf("missing activity marker text in view, got:\n%s", view)
	}
	// must show time labels
	if !strings.Contains(view, ":00") {
		t.Fatalf("missing time labels, got:\n%s", view)
	}
}

func TestDayViewInspectorShowsActivityDetail(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	// create slots at a time that maps to 09:00 local
	slotUTC := time.Date(2026, 4, 3, 9, 0, 0, 0, time.Local).UTC().Truncate(15 * time.Minute)
	if err := store.UpsertActivitySlots(ctx, []model.ActivitySlot{
		{SlotTime: slotUTC, Operator: "claude-code", MsgCount: 12, FirstText: "refactor auth", Cwd: "/Users/akoskm/Projects/hrs"},
		{SlotTime: slotUTC, Operator: "codex", MsgCount: 3, FirstText: "other work", Cwd: "/Users/akoskm/Projects/elaiia"},
	}); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	m.daySlotStart = time.Date(2026, 4, 3, 9, 0, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	m.dayFocusKind = "slot"
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 30})
	app := updated.(AppModel)
	view := stripANSI(app.View())

	// inspector should show operator-grouped activity
	if !strings.Contains(view, "claude-code") {
		t.Fatalf("inspector missing claude-code operator, got:\n%s", view)
	}
	if !strings.Contains(view, "12 msgs") {
		t.Fatalf("inspector missing message count, got:\n%s", view)
	}
}

func TestDayViewInspectorPanelRendersOnRight(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Morning standup", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app := updated.(AppModel)
	view := stripANSI(app.View())

	// inspector must show on right side with its tabs
	if !strings.Contains(view, "Overview") {
		t.Fatalf("inspector missing Overview tab, got:\n%s", view)
	}
	if !strings.Contains(view, "Actions") {
		t.Fatalf("inspector missing Actions tab, got:\n%s", view)
	}
	// timeline and inspector must coexist — check both time labels and inspector content
	if !strings.Contains(view, ":00") {
		t.Fatalf("missing time labels alongside inspector, got:\n%s", view)
	}
	// verify inspector shows entry info (entry is focused)
	if !strings.Contains(view, "Morning standup") {
		t.Fatalf("inspector not showing focused entry description, got:\n%s", view)
	}
}

func TestDayViewFullUIElements(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Code review", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 30, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	// place activity slot at 14:00 local — within the visible window (entry is at 11:00-12:30 local)
	slotUTC := time.Date(2026, 4, 3, 14, 0, 0, 0, time.Local).UTC().Truncate(15 * time.Minute)
	if err := store.UpsertActivitySlots(ctx, []model.ActivitySlot{
		{SlotTime: slotUTC, Operator: "claude-code", MsgCount: 7, FirstText: "implement auth", Cwd: "/tmp/project"},
	}); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(app.View())

	checks := map[string]string{
		"header":          "hrs",
		"timeline title":  "Timeline",
		"date header":     "2026-04-03",
		"time column":     "time",
		"activity column": "activity",
		"time label":      ":00",
		"entry text":      "Code review",
		"activity marker": "implement auth",
		"subheader slots": "active slots",
		"inspector tab":   "Overview",
		"actions tab":     "Actions",
		"status bar day":  "day 2026-04-03",
		"status bar keys": "j/k items",
	}
	for name, want := range checks {
		if !strings.Contains(view, want) {
			t.Errorf("missing %s (%q) in view:\n%s", name, want, view)
		}
	}
}
