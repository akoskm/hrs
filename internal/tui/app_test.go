package tui

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"

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

func TestRenderDialogTextInputDoesNotInsertLeadingSpaceForVisibleCaret(t *testing.T) {
	rendered := stripANSI(renderDialogTextInput("production bug", 0, true, true, 40))
	if rendered != "> production bug" {
		t.Fatalf("rendered input = %q, want %q", rendered, "> production bug")
	}
}

func TestTextWithCaretAtEndDoesNotAddExtraCell(t *testing.T) {
	rendered := stripANSI(textWithCaret("Auth", true, true))
	if rendered != "Auth" {
		t.Fatalf("textWithCaret(end) = %q, want %q", rendered, "Auth")
	}
}

func TestRenderDialogTextInputShowsCaretAfterLastCharacter(t *testing.T) {
	rendered := stripANSI(renderDialogTextInput("DELTA-838 reviews", len([]rune("DELTA-838 reviews")), true, true, 40))
	if rendered != "> DELTA-838 reviews▏" {
		t.Fatalf("rendered input = %q, want %q", rendered, "> DELTA-838 reviews▏")
	}
}

func TestRenderDialogTextInputLongTextDoesNotShiftOnBlinkAtEnd(t *testing.T) {
	text := "investigate production bug with cache miss"
	hidden := stripANSI(renderDialogTextInput(text, len([]rune(text)), false, true, 20))
	visible := stripANSI(renderDialogTextInput(text, len([]rune(text)), true, true, 20))
	if strings.TrimRight(hidden, " ") != strings.TrimSuffix(visible, "▏") {
		t.Fatalf("long input shifts on blink: hidden=%q visible=%q", hidden, visible)
	}
}

func TestEntryEditDialogLongDescriptionCanScrollBackToBeginning(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	desc := "invetigate production bug with what it seems to be a deduplication"
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: desc, StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	app.width = 50
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	for i := 0; i < len([]rune(desc)); i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
		app = updated.(AppModel)
	}
	view := stripANSI(renderEntryEditDialog(app, newStyles(app.width), ""))
	if !strings.Contains(view, "invetigate") {
		t.Fatalf("dialog view missing beginning after scrolling left, got:\n%s", view)
	}
}

func TestEntryEditDialogHomeEndMoveCursorToBounds(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	desc := "invetigate production bug with what it seems to be a deduplication"
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: desc, StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyHome})
	app = updated.(AppModel)
	if app.entryInputCursor != 0 {
		t.Fatalf("entryInputCursor after home = %d, want 0", app.entryInputCursor)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnd})
	app = updated.(AppModel)
	if app.entryInputCursor != len([]rune(desc)) {
		t.Fatalf("entryInputCursor after end = %d, want %d", app.entryInputCursor, len([]rune(desc)))
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

func TestManageProjectsShowsProjectColorCue(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	color := "#20c997"
	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Color: color, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	app := updated.(AppModel)
	view := stripANSI(app.View())

	if !strings.Contains(view, "[billable] Elaiia (elaiia)") {
		t.Fatalf("manage dialog missing project color cue: %q", view)
	}
	if !strings.Contains(view, "Elaiia | default billable") {
		t.Fatalf("manage dialog missing project summary: %q", view)
	}
	if !strings.Contains(view, "Color: #20c997") {
		t.Fatalf("manage dialog missing separate color line: %q", view)
	}
}

func TestManageTimeOffCanCreateCustomTypeForProject(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	alpha, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "USD"})
	if err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Beta", Code: "beta", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject(beta) error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	app := updated.(AppModel)
	if app.mode != modeTimeOff || app.timeOffDialogMode != timeOffDialogManage {
		t.Fatalf("dialog state = %q/%q, want time-off/manage", app.mode, app.timeOffDialogMode)
	}
	if !strings.Contains(stripANSI(app.View()), "Manage Time Off") {
		t.Fatalf("view missing manage time off dialog: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	app = updated.(AppModel)
	if app.timeOffDialogMode != timeOffDialogCreate {
		t.Fatalf("timeOffDialogMode = %q, want create", app.timeOffDialogMode)
	}
	for _, r := range []rune("Conference Leave") {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.timeOffDialogMode != timeOffDialogManage {
		t.Fatalf("timeOffDialogMode after create = %q, want manage", app.timeOffDialogMode)
	}

	types, err := store.ListTimeOffTypesByProject(ctx, alpha.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject(alpha) error = %v", err)
	}
	if len(types) != 4 {
		t.Fatalf("len(types) = %d, want 4", len(types))
	}
	if types[len(types)-1].Name != "Vacation" {
		t.Fatalf("types = %#v, want seeded + custom type", types)
	}
	if !strings.Contains(stripANSI(app.View()), "Conference Leave") {
		t.Fatalf("view missing new custom type: %q", stripANSI(app.View()))
	}
}

func TestProjectColorIndicatorUsesForegroundColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
	})

	color := "#20c997"
	indicator := projectColorIndicator(model.Project{Color: &color})
	if stripANSI(indicator) != "#20c997" {
		t.Fatalf("indicator text = %q, want %q", stripANSI(indicator), "#20c997")
	}
	if !regexp.MustCompile(`\x1b\[[0-9;]*38;`).MatchString(indicator) {
		t.Fatalf("indicator missing foreground color escape: %q", indicator)
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

func TestEditDialogRejectsInvalidTimeSuffix(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "p",
		Description:  "Auth",
		StartedAt:    time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 11, 30, 0, 0, time.UTC),
	}); err != nil {
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
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	if app.entryInputField != "end" {
		t.Fatalf("field = %q, want end", app.entryInputField)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-33404")})
	app = updated.(AppModel)
	wantEnd := clock(time.Date(2026, 4, 3, 11, 30, 0, 0, time.UTC))
	if app.entryEndInput != wantEnd {
		t.Fatalf("entryEndInput = %q, want %s", app.entryEndInput, wantEnd)
	}
	if app.err != nil {
		t.Fatalf("err = %v, want nil", app.err)
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
	got := textWithCaret("Auth", true, true)
	if !strings.HasPrefix(stripANSI(got), "Auth") {
		t.Fatalf("textWithCaret() = %q, want visible end caret on Auth", stripANSI(got))
	}
	if got := textWithCaret("Auth", false, true); got != "Auth" {
		t.Fatalf("textWithCaret(hidden) = %q, want 'Auth'", got)
	}
	if got := textWithCaret("Auth", true, false); got != "Auth" {
		t.Fatalf("textWithCaret(inactive) = %q, want 'Auth'", got)
	}
	if got := textWithCaretAt("Auth", 0, true, true); stripANSI(got) != "Auth" {
		t.Fatalf("textWithCaretAt() = %q, want no inserted leading cell", stripANSI(got))
	}
}

func TestDialogTextViewportClipsFromLeft(t *testing.T) {
	got := dialogTextViewport("investigate production bug with what it seems to be a cache miss", len([]rune("investigate production bug with what it seems to be a cache miss")), true, true, 20)
	plain := stripANSI(got)
	if strings.Contains(plain, "...") {
		t.Fatalf("dialogTextViewport() = %q, should not contain ellipsis", got)
	}
	if !strings.Contains(plain, "cache miss") {
		t.Fatalf("dialogTextViewport() = %q, want tail text visible", got)
	}
	if lipgloss.Width(plain) > 20 {
		t.Fatalf("dialogTextViewport() = %q, want clipped width <= 20", plain)
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

func TestRenderVerticalEntryCellShowsLabelForUnfocusedSingleSlot(t *testing.T) {
	slotStart := time.Date(2026, 4, 3, 11, 0, 0, 0, time.Local)
	slotEnd := slotStart.Add(15 * time.Minute)
	styles := newStyles(80)
	rendered := stripANSI(renderVerticalEntryCell(slotStart, slotStart, slotEnd, slotStart, slotEnd, false, false, false, 5, "test", styles.confirmed, styles))
	if !strings.Contains(rendered, "test") {
		t.Fatalf("unfocused single-slot cell = %q, want label visible", rendered)
	}
}

func TestOutlinedBlockCellSingleSlotPrefersLabelOverBorders(t *testing.T) {
	slotStart := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)
	slotEnd := slotStart.Add(15 * time.Minute)
	got := outlinedBlockCell(slotStart, slotEnd, slotStart, slotEnd, 5, "hello")
	if got != "hello" {
		t.Fatalf("single-slot cell = %q, want %q", got, "hello")
	}
}

func TestOutlinedBlockCellKeepsBottomBorderWhenShortBlockLabelMovesToTop(t *testing.T) {
	itemStart := time.Date(2026, 4, 9, 10, 50, 0, 0, time.Local)
	itemEnd := time.Date(2026, 4, 9, 11, 10, 0, 0, time.Local)
	slotStart := time.Date(2026, 4, 9, 11, 0, 0, 0, time.Local)
	slotEnd := time.Date(2026, 4, 9, 11, 10, 0, 0, time.Local)

	got := outlinedBlockCellWithViewport(slotStart, slotEnd, time.Time{}, itemStart, itemEnd, false, false, 24, "verify hotfix in prod")
	if strings.Contains(got, "verify hotfix in prod") {
		t.Fatalf("ending midpoint cell = %q, want label only on top border row", got)
	}
	if got != "└──────────────────────┘" {
		t.Fatalf("ending midpoint cell = %q, want preserved bottom border", got)
	}
}

func TestOutlinedBlockCellShowsLabelOnStartingMidpointRow(t *testing.T) {
	itemStart := time.Date(2026, 4, 9, 13, 0, 0, 0, time.Local)
	itemEnd := time.Date(2026, 4, 9, 13, 20, 0, 0, time.Local)
	slotStart := time.Date(2026, 4, 9, 13, 0, 0, 0, time.Local)
	slotEnd := time.Date(2026, 4, 9, 13, 15, 0, 0, time.Local)

	got := outlinedBlockCellWithViewport(slotStart, slotEnd, time.Time{}, itemStart, itemEnd, false, false, 24, "check why language")
	want := "┌─check why language───┐"
	if got != want {
		t.Fatalf("starting midpoint cell = %q, want %q", got, want)
	}
}

func TestCenteredBlockLabelUsesFullWidth(t *testing.T) {
	got := centeredBlockLabel("hello", 5)
	if got != "hello" {
		t.Fatalf("centeredBlockLabel() = %q, want %q", got, "hello")
	}
}

func TestTruncateForWidthUsesDisplayWidth(t *testing.T) {
	got := truncateForWidth("│   │", 5)
	if got != "│   │" {
		t.Fatalf("truncateForWidth() = %q, want %q", got, "│   │")
	}
}

func TestTruncateForWidthClipsWideRunesAtNarrowWidths(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
	}{
		{name: "single wide width one", text: "界", width: 1},
		{name: "single wide width two", text: "界", width: 2},
		{name: "two wide width three", text: "界界", width: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateForWidth(tt.text, tt.width)
			if lipgloss.Width(got) > tt.width {
				t.Fatalf("truncateForWidth(%q, %d) width = %d, want <= %d; got %q", tt.text, tt.width, lipgloss.Width(got), tt.width, got)
			}
		})
	}
}

func TestOutlinedBlockCellAnchorsViewportTopLabel(t *testing.T) {
	viewportStart := time.Date(2026, 4, 7, 7, 0, 0, 0, time.Local)
	itemStart := viewportStart
	itemEnd := viewportStart.Add(90 * time.Minute)
	firstSlotStart := viewportStart
	firstSlotEnd := firstSlotStart.Add(20 * time.Minute)
	midSlotStart := viewportStart.Add(40 * time.Minute)
	midSlotEnd := midSlotStart.Add(20 * time.Minute)

	if got := outlinedBlockCellWithViewport(firstSlotStart, firstSlotEnd, viewportStart, itemStart, itemEnd, false, false, 24, "improve TUI experience"); strings.Contains(got, "improve TUI experience") {
		t.Fatalf("first visible cell = %q, want plain top border for multi-row viewport-top entry", got)
	}
	if got := outlinedBlockCellWithViewport(midSlotStart, midSlotEnd, viewportStart, itemStart, itemEnd, false, false, 24, "improve TUI experience"); !strings.Contains(got, "improve TUI experience") {
		t.Fatalf("mid cell = %q, want label in body row", got)
	}
}

func TestOutlinedBlockCellSharedBoundaryForTouchingEntries(t *testing.T) {
	upperStart := time.Date(2026, 4, 3, 13, 0, 0, 0, time.UTC)
	boundary := upperStart.Add(75 * time.Minute)
	upperSlotStart := boundary.Add(-15 * time.Minute)
	upperSlotEnd := boundary
	lowerSlotStart := boundary
	lowerSlotEnd := boundary.Add(15 * time.Minute)
	lowerEnd := boundary.Add(75 * time.Minute)

	if got := outlinedBlockCellWithViewport(upperSlotStart, upperSlotEnd, time.Time{}, upperStart, boundary, false, true, 12, "upper"); strings.Contains(got, "├") || strings.Contains(got, "┤") {
		t.Fatalf("upper touching cell = %q, want no duplicate shared boundary", got)
	}
	if got := outlinedBlockCellWithViewport(lowerSlotStart, lowerSlotEnd, time.Time{}, boundary, lowerEnd, true, false, 12, "lower"); !strings.Contains(got, "├") || !strings.Contains(got, "┤") {
		t.Fatalf("lower touching cell = %q, want shared boundary on lower start row", got)
	}
}

func TestOutlinedBlockCellTouchingShortBlockWithoutInteriorRowUsesSharedBoundaryLabel(t *testing.T) {
	boundary := time.Date(2026, 4, 3, 20, 0, 0, 0, time.Local)
	itemStart := boundary
	itemEnd := boundary.Add(30 * time.Minute)
	firstSlotStart := boundary
	firstSlotEnd := boundary.Add(15 * time.Minute)
	lastSlotStart := boundary.Add(15 * time.Minute)
	lastSlotEnd := itemEnd

	if got := outlinedBlockCellWithViewport(firstSlotStart, firstSlotEnd, time.Time{}, itemStart, itemEnd, true, false, 24, "fix e2e test suite"); !strings.Contains(got, "fix e2e test suite") || !strings.Contains(got, "├") {
		t.Fatalf("first touching short cell = %q, want label on shared boundary", got)
	}
	if got := outlinedBlockCellWithViewport(lastSlotStart, lastSlotEnd, time.Time{}, itemStart, itemEnd, true, false, 24, "fix e2e test suite"); strings.Contains(got, "fix e2e test suite") {
		t.Fatalf("last touching short cell = %q, want no duplicate label", got)
	}
}

func TestRenderActivityCellKeepsBorderBetweenDifferentProjects(t *testing.T) {
	projectA := "alpha"
	projectB := "beta"
	boundary := time.Date(2026, 4, 7, 14, 30, 0, 0, time.Local)
	upperEnd := boundary
	lowerEnd := boundary.Add(45 * time.Minute)
	entries := []model.TimeEntryDetail{
		{
			TimeEntry: model.TimeEntry{
				ID:        "upper",
				ProjectID: &projectA,
				StartedAt: boundary.Add(-45 * time.Minute),
				EndedAt:   &upperEnd,
				Status:    model.StatusConfirmed,
			},
			ProjectName: "Alpha",
		},
		{
			TimeEntry: model.TimeEntry{
				ID:        "lower",
				ProjectID: &projectB,
				StartedAt: boundary,
				EndedAt:   &lowerEnd,
				Status:    model.StatusConfirmed,
			},
			ProjectName: "Beta",
		},
	}

	app := AppModel{entries: entries}
	styles := newStyles(80)
	got := stripANSI(renderActivityCell(app, time.Time{}, boundary, boundary.Add(15*time.Minute), entries, 24, styles))
	if !strings.Contains(got, "┌") || strings.Contains(got, "├") {
		t.Fatalf("lower cross-project cell = %q, want fresh top border", got)
	}
}

func TestRenderActivityCellKeepsBorderBetweenDifferentDescriptions(t *testing.T) {
	project := "alpha"
	upperDesc := "migration follow up for multiple datasets"
	lowerDesc := "michal interview"
	boundary := time.Date(2026, 4, 7, 14, 30, 0, 0, time.Local)
	upperEnd := boundary
	lowerEnd := boundary.Add(45 * time.Minute)
	entries := []model.TimeEntryDetail{
		{
			TimeEntry: model.TimeEntry{
				ID:          "upper",
				ProjectID:   &project,
				Description: &upperDesc,
				StartedAt:   boundary.Add(-45 * time.Minute),
				EndedAt:     &upperEnd,
				Status:      model.StatusConfirmed,
			},
			ProjectName: "Alpha",
		},
		{
			TimeEntry: model.TimeEntry{
				ID:          "lower",
				ProjectID:   &project,
				Description: &lowerDesc,
				StartedAt:   boundary,
				EndedAt:     &lowerEnd,
				Status:      model.StatusConfirmed,
			},
			ProjectName: "Alpha",
		},
	}

	app := AppModel{entries: entries}
	styles := newStyles(80)

	upper := stripANSI(renderActivityCell(app, time.Time{}, boundary.Add(-15*time.Minute), boundary, entries, 30, styles))
	if !strings.Contains(upper, "└") {
		t.Fatalf("upper cell = %q, want closing border when descriptions differ", upper)
	}

	lower := stripANSI(renderActivityCell(app, time.Time{}, boundary, boundary.Add(15*time.Minute), entries, 30, styles))
	if !strings.Contains(lower, "┌") || strings.Contains(lower, "├") {
		t.Fatalf("lower cell = %q, want fresh top border when descriptions differ", lower)
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

func TestTimelineMonthViewToggleRestoresPreviousView(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Work", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	model.width = 125
	model.height = 30

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app := updated.(AppModel)
	if app.timelineView != timelineViewMonth {
		t.Fatalf("timelineView after m = %q, want month", app.timelineView)
	}
	if !strings.Contains(stripANSI(app.View()), "Mon Tue Wed Thu Fri Sat Sun") {
		t.Fatalf("month view missing weekday header: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app = updated.(AppModel)
	if app.timelineView != timelineViewDay {
		t.Fatalf("timelineView after second m = %q, want day", app.timelineView)
	}

	model2, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model2.width = 120
	model2.height = 30
	updated, _ = model2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app = updated.(AppModel)
	if app.timelineView != timelineViewList {
		t.Fatalf("list toggle restore = %q, want list", app.timelineView)
	}
}

func TestTimelineMonthViewNavigationAndEnter(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	today := dayStart(time.Now().UTC())
	targetDay := today.AddDate(0, 0, -10)
	weekEarlier := targetDay.AddDate(0, 0, -7)

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Chosen day", StartedAt: targetDay.Add(9 * time.Hour), EndedAt: targetDay.Add(11 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Week earlier", StartedAt: weekEarlier.Add(9 * time.Hour), EndedAt: weekEarlier.Add(10 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	model.width = 120
	model.height = 30
	model.dayDate = targetDay

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app := updated.(AppModel)
	if !app.displayedDay().Equal(targetDay) {
		t.Fatalf("selected month day = %s, want %s", app.displayedDay().Format("2006-01-02"), targetDay.Format("2006-01-02"))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(targetDay.AddDate(0, 0, -1)) {
		t.Fatalf("day after h = %s, want %s", app.displayedDay().Format("2006-01-02"), targetDay.AddDate(0, 0, -1).Format("2006-01-02"))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(targetDay.AddDate(0, 0, 6)) {
		t.Fatalf("day after j = %s, want %s", app.displayedDay().Format("2006-01-02"), targetDay.AddDate(0, 0, 6).Format("2006-01-02"))
	}

	app.dayDate = targetDay
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.timelineView != timelineViewDay {
		t.Fatalf("timelineView after enter = %q, want day", app.timelineView)
	}
	if !app.displayedDay().Equal(targetDay) {
		t.Fatalf("displayedDay after enter = %s, want %s", app.displayedDay().Format("2006-01-02"), targetDay.Format("2006-01-02"))
	}
	if got := descriptionOrID(app.entries[app.cursor]); got != "Chosen day" {
		t.Fatalf("focused entry after enter = %q, want Chosen day", got)
	}
}

func TestTimelineMonthViewCanRecordProjectTimeOff(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	targetDay := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)

	alpha, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "USD"})
	if err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Beta", Code: "beta", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject(beta) error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	model.width = 120
	model.height = 30
	model.dayDate = targetDay

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	app = updated.(AppModel)
	if app.mode != modeTimeOff || app.timeOffDialogMode != timeOffDialogRecord {
		t.Fatalf("dialog state = %q/%q, want time-off/record", app.mode, app.timeOffDialogMode)
	}
	if !strings.Contains(stripANSI(app.View()), "Record Time Off") {
		t.Fatalf("view missing record time off dialog: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	for i := 0; i < 3; i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeTimeline {
		t.Fatalf("mode after save = %q, want timeline", app.mode)
	}

	records, err := store.ListTimeOffDaysInRange(ctx, "2026-04-01", "2026-04-30")
	if err != nil {
		t.Fatalf("ListTimeOffDaysInRange() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].ProjectID != alpha.ID {
		t.Fatalf("records[0].ProjectID = %q, want %q", records[0].ProjectID, alpha.ID)
	}
	if records[0].Day != "2026-04-15" {
		t.Fatalf("records[0].Day = %q, want 2026-04-15", records[0].Day)
	}
	if records[0].TimeOffType != "Vacation" {
		t.Fatalf("records[0].TimeOffType = %q, want Vacation", records[0].TimeOffType)
	}

	view := stripANSI(app.View())
	if !strings.Contains(view, "Vacation @ Alpha") {
		t.Fatalf("month view missing recorded time off: %q", view)
	}
}

func TestTimelineMonthViewCanMoveIntoFuture(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.width = 120
	model.height = 30
	model.dayDate = dayStart(time.Now()).AddDate(0, 0, 1)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app := updated.(AppModel)
	future := dayStart(time.Now()).AddDate(0, 0, 2)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(future) {
		t.Fatalf("displayedDay after future move = %s, want %s", app.displayedDay().Format("2006-01-02"), future.Format("2006-01-02"))
	}
}

func TestRecordTimeOffDialogDateRangeIsEditable(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}

	targetDay := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	model.width = 120
	model.height = 30
	model.dayDate = targetDay

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	app = updated.(AppModel)
	view := stripANSI(app.View())
	if !strings.Contains(view, "2026-04-15") {
		t.Fatalf("dialog missing initial date range: %q", view)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	for i := 0; i < len("2026-04-15"); i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		app = updated.(AppModel)
	}
	for _, r := range []rune("2026-04-20") {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	for i := 0; i < len("2026-04-15"); i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		app = updated.(AppModel)
	}
	for _, r := range []rune("2026-04-22") {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	for i := 0; i < 3; i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)

	records, err := store.ListTimeOffDaysInRange(ctx, "2026-04-01", "2026-04-30")
	if err != nil {
		t.Fatalf("ListTimeOffDaysInRange() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("len(records) = %d, want 3", len(records))
	}
	gotDays := []string{records[0].Day, records[1].Day, records[2].Day}
	wantDays := []string{"2026-04-20", "2026-04-21", "2026-04-22"}
	for i := range wantDays {
		if gotDays[i] != wantDays[i] {
			t.Fatalf("record days = %v, want %v", gotDays, wantDays)
		}
		if records[i].TimeOffType != "Vacation" {
			t.Fatalf("records[%d].TimeOffType = %q, want Vacation", i, records[i].TimeOffType)
		}
	}
}

func TestRecordTimeOffDialogClearRemovesSelectedRangeForProject(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	alpha, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "USD"})
	if err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	beta, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Beta", Code: "beta", Currency: "USD"})
	if err != nil {
		t.Fatalf("CreateProject(beta) error = %v", err)
	}
	if err := store.EnsureProjectDefaultTimeOffTypes(ctx, alpha.ID); err != nil {
		t.Fatalf("EnsureProjectDefaultTimeOffTypes(alpha) error = %v", err)
	}
	if err := store.EnsureProjectDefaultTimeOffTypes(ctx, beta.ID); err != nil {
		t.Fatalf("EnsureProjectDefaultTimeOffTypes(beta) error = %v", err)
	}
	alphaTypes, err := store.ListTimeOffTypesByProject(ctx, alpha.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject(alpha) error = %v", err)
	}
	betaTypes, err := store.ListTimeOffTypesByProject(ctx, beta.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject(beta) error = %v", err)
	}
	for _, day := range []string{"2026-04-20", "2026-04-21", "2026-04-22"} {
		if _, err := store.UpsertTimeOffDay(ctx, alpha.ID, alphaTypes[2].ID, day); err != nil {
			t.Fatalf("UpsertTimeOffDay(alpha,%s) error = %v", day, err)
		}
		if _, err := store.UpsertTimeOffDay(ctx, beta.ID, betaTypes[0].ID, day); err != nil {
			t.Fatalf("UpsertTimeOffDay(beta,%s) error = %v", day, err)
		}
	}

	targetDay := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	model.width = 120
	model.height = 30
	model.dayDate = targetDay

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	for i := 0; i < len("2026-04-20"); i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		app = updated.(AppModel)
	}
	for _, r := range []rune("2026-04-22") {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	for i := 0; i < 3; i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyUp})
		app = updated.(AppModel)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)

	records, err := store.ListTimeOffDaysInRange(ctx, "2026-04-20", "2026-04-22")
	if err != nil {
		t.Fatalf("ListTimeOffDaysInRange() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("len(records) = %d, want 3 remaining beta records", len(records))
	}
	for _, record := range records {
		if record.ProjectID != beta.ID {
			t.Fatalf("record.ProjectID = %q, want only beta %q", record.ProjectID, beta.ID)
		}
	}
}

func TestRecordTimeOffDialogPreloadsExistingRangeForEditing(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	alpha, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "USD"})
	if err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Beta", Code: "beta", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject(beta) error = %v", err)
	}
	if err := store.EnsureProjectDefaultTimeOffTypes(ctx, alpha.ID); err != nil {
		t.Fatalf("EnsureProjectDefaultTimeOffTypes(alpha) error = %v", err)
	}
	types, err := store.ListTimeOffTypesByProject(ctx, alpha.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject(alpha) error = %v", err)
	}
	for _, day := range []string{"2026-04-20", "2026-04-21", "2026-04-22"} {
		if _, err := store.UpsertTimeOffDay(ctx, alpha.ID, types[2].ID, day); err != nil {
			t.Fatalf("UpsertTimeOffDay(alpha,%s) error = %v", day, err)
		}
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	model.width = 120
	model.height = 30
	model.dayDate = time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	app = updated.(AppModel)

	if project := app.selectedTimeOffProject(); project == nil || project.ID != alpha.ID {
		t.Fatalf("selected project = %#v, want alpha", project)
	}
	if got := app.selectedTimeOffType(); got == nil || got.Name != "Vacation" {
		t.Fatalf("selected time off type = %#v, want Vacation", got)
	}
	if app.timeOffFromInput != "2026-04-20" {
		t.Fatalf("timeOffFromInput = %q, want 2026-04-20", app.timeOffFromInput)
	}
	if app.timeOffToInput != "2026-04-22" {
		t.Fatalf("timeOffToInput = %q, want 2026-04-22", app.timeOffToInput)
	}
	view := stripANSI(app.View())
	if !strings.Contains(view, "2026-04-20 to 2026-04-22 | Alpha") {
		t.Fatalf("dialog summary not preloaded for edit: %q", view)
	}
}

func TestTimelineMonthViewShowsProjectTotalsAndOverflow(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	targetDay := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	projects := []struct {
		name string
		code string
		dur  time.Duration
	}{
		{name: "Alpha", code: "alpha", dur: 3 * time.Hour},
		{name: "Beta", code: "beta", dur: 2 * time.Hour},
		{name: "Gamma", code: "gamma", dur: time.Hour},
		{name: "Delta", code: "delta", dur: 30 * time.Minute},
	}
	startHour := 8
	for _, project := range projects {
		if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: project.name, Code: project.code, HourlyRate: 15000, Currency: "CHF"}); err != nil {
			t.Fatalf("CreateProject() error = %v", err)
		}
		if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: project.code, Description: project.name + " work", StartedAt: targetDay.Add(time.Duration(startHour) * time.Hour), EndedAt: targetDay.Add(time.Duration(startHour)*time.Hour + project.dur)}); err != nil {
			t.Fatalf("CreateManualEntry() error = %v", err)
		}
		startHour += 3
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.width = 90
	model.height = 30
	model.dayDate = targetDay

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app := updated.(AppModel)
	view := stripANSI(app.View())
	for _, want := range []string{"Apr 2026", "15", "6h30m", "Alpha 3h", "Beta 2h", "+2 more"} {
		if !strings.Contains(view, want) {
			t.Fatalf("month view missing %q: %q", want, view)
		}
	}
}

func TestTimelineMonthViewFitsViewportWidth(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	targetDay := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.width = 120
	model.height = 30
	model.dayDate = targetDay

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app := updated.(AppModel)
	for _, line := range strings.Split(strings.TrimRight(stripANSI(app.View()), "\n"), "\n") {
		if lipgloss.Width(line) > model.width {
			t.Fatalf("month view line width = %d, want <= %d\n%s", lipgloss.Width(line), model.width, line)
		}
	}
}

func TestTimelineMonthViewUsesAvailableWidth(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	targetDay := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.width = 120
	model.height = 30
	model.dayDate = targetDay

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app := updated.(AppModel)
	lines := strings.Split(strings.TrimRight(stripANSI(app.View()), "\n"), "\n")
	maxWidth := 0
	for _, line := range lines {
		if !strings.Contains(line, "┌") {
			continue
		}
		if w := lipgloss.Width(line); w > maxWidth {
			maxWidth = w
		}
	}
	if maxWidth == 0 {
		t.Fatalf("month view missing bordered grid rows")
	}
	if maxWidth != model.width {
		t.Fatalf("month view max line width = %d, want exact viewport width %d", maxWidth, model.width)
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
	model.height = 30
	model.daySlotSpan = 15 * time.Minute
	model.daySlotStart = maxSlotStartForDay(model.displayedDay(), model.daySlotSpan).Add(-time.Hour)
	start := model.daySlotStart
	model.moveSlot(15*time.Minute, 15*time.Minute)
	delta := model.daySlotStart.Sub(start)
	if delta <= 0 {
		t.Fatalf("slot should move forward, got delta = %s", delta)
	}
	model.moveSlot(-time.Hour, time.Hour)
	if model.daySlotSpan <= 0 {
		t.Fatalf("slot span should be positive after shift+up, got %s", model.daySlotSpan)
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
	model.height = 30
	today := dayStart(time.Now())
	model.dayDate = today
	model.daySlotSpan = 15 * time.Minute
	// position at last row and try to move forward — should stay on today
	dayEntries := dayEntriesForDate(model.entries, today.Format("2006-01-02"))
	window := dayTimelineWindow(dayEntries, today, model.dayWindowStart)
	rows := dayTimelineRows(window, model.height)
	lastRow := rows[len(rows)-1]
	model.daySlotStart = lastRow.start
	model.moveSlot(15*time.Minute, 15*time.Minute)
	if !model.displayedDay().Equal(today) {
		t.Fatalf("displayedDay = %s, want today", model.displayedDay().Format("2006-01-02"))
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
	day := time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local)

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Morning", StartedAt: day.Add(9 * time.Hour), EndedAt: day.Add(10 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Noon", StartedAt: day.Add(12 * time.Hour), EndedAt: day.Add(13 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.SetDefaultTimelineView("day")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	app := updated.(AppModel)
	// Position slot in a guaranteed gap between the seeded entries.
	app.daySlotStart = day.Add(10*time.Hour + 30*time.Minute)
	app.daySlotSpan = time.Hour
	app.dayFocusKind = "slot"

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
			if formatRange(entry.StartedAt, entry.EndedAt) != "10:30-11:30" {
				t.Fatalf("created range = %s, want 10:30-11:30", formatRange(entry.StartedAt, entry.EndedAt))
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
	first := renderStatusBar(model, 120)
	updated, cmd := model.Update(spinner.TickMsg{ID: model.syncSpinner.ID()})
	model = updated.(AppModel)
	second := renderStatusBar(model, 120)
	if !strings.Contains(first, "Syncing") {
		t.Fatalf("first status missing sync bar: %q", first)
	}
	if !strings.Contains(second, "Syncing") {
		t.Fatalf("second status missing sync bar: %q", second)
	}
	if first == second {
		t.Fatalf("sync bar did not animate: first=%q second=%q", first, second)
	}
	if cmd == nil {
		t.Fatal("cmd = nil after sync pulse, want follow-up tick")
	}
	model.syncing = false
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	app := updated.(AppModel)
	if !app.syncing {
		t.Fatal("syncing = false after s, want true")
	}
	if cmd == nil {
		t.Fatal("cmd = nil after s, want sync commands")
	}
	updated, _ = model.Update(syncDoneMsg{})
	app = updated.(AppModel)
	if app.syncing {
		t.Fatal("syncing = true, want false")
	}
}

func TestSyncStatusBarShowsErrorsWithoutSpinner(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	model, err := NewAppModelWithSync(ctx, store, func() error { return nil })
	if err != nil {
		t.Fatalf("NewAppModelWithSync() error = %v", err)
	}
	model.width = 120
	model.height = 20

	updated, _ := model.Update(syncDoneMsg{err: errors.New("boom")})
	app := updated.(AppModel)
	status := renderStatusBar(app, 120)
	if !strings.Contains(status, "Sync Error") {
		t.Fatalf("status missing sync error label: %q", status)
	}
	if !strings.Contains(status, "boom") {
		t.Fatalf("status missing sync error text: %q", status)
	}
	if app.syncing {
		t.Fatal("syncing = true after sync error, want false")
	}
}

func TestSyncingViewDoesNotShowTrailingEllipsis(t *testing.T) {
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

	view := stripANSI(model.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("view has no lines")
	}
	status := lines[len(lines)-1]
	if !strings.Contains(status, "Syncing") {
		t.Fatalf("status missing Syncing: %q", status)
	}
	if strings.HasSuffix(status, "...") {
		t.Fatalf("status has trailing ellipsis: %q", status)
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

func TestTimelineCanOpenReportView(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := time.Now().In(time.Local)
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "Reportable work",
		StartedAt:    start,
		EndedAt:      start.Add(2 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	view := stripANSI(app.View())
	if !strings.Contains(view, "Summary") || !strings.Contains(view, "By project") {
		t.Fatalf("report view missing summary sections: %q", view)
	}
	if !strings.Contains(view, "Elaiia") {
		t.Fatalf("report view missing project data: %q", view)
	}
}

func TestTimelineCanOpenDashboardView(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := time.Now().In(time.Local)
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "Reportable work",
		StartedAt:    start,
		EndedAt:      start.Add(2 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if err := store.EnsureProjectDefaultTimeOffTypes(ctx, project.ID); err != nil {
		t.Fatalf("EnsureProjectDefaultTimeOffTypes() error = %v", err)
	}
	types, err := store.ListTimeOffTypesByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject() error = %v", err)
	}
	var vacationID string
	for _, item := range types {
		if item.Name == "Vacation" {
			vacationID = item.ID
			break
		}
	}
	if vacationID == "" {
		t.Fatal("vacation type not found")
	}
	futureDay := now.AddDate(0, 0, 10).Format("2006-01-02")
	if _, err := store.UpsertTimeOffDay(ctx, project.ID, vacationID, futureDay); err != nil {
		t.Fatalf("UpsertTimeOffDay() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 28})
	app := updated.(AppModel)

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	app = updated.(AppModel)
	if app.mode != modeDashboard {
		t.Fatalf("mode = %q, want dashboard", app.mode)
	}
	view := stripANSI(app.View())
	for _, want := range []string{"Dashboard", "Year progress", "Planned time off", "Daily activity", "Vacation"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dashboard view missing %q: %q", want, view)
		}
	}
	if !strings.Contains(view, now.Format("Jan")) {
		t.Fatalf("dashboard view missing month label: %q", view)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)
	if app.mode != modeTimeline {
		t.Fatalf("mode after esc = %q, want timeline", app.mode)
	}
}

func TestDashboardNavigationUpdatesSelectedDay(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := dayStart(time.Now().In(time.Local))
	for _, tc := range []struct {
		desc  string
		start time.Time
		end   time.Time
	}{
		{desc: "Yesterday work", start: now.AddDate(0, 0, -1).Add(8 * time.Hour), end: now.AddDate(0, 0, -1).Add(9 * time.Hour)},
		{desc: "Today work", start: now.Add(9 * time.Hour), end: now.Add(10 * time.Hour)},
		{desc: "Tomorrow work", start: now.AddDate(0, 0, 1).Add(11 * time.Hour), end: now.AddDate(0, 0, 1).Add(12 * time.Hour)},
		{desc: "Next week work", start: now.AddDate(0, 0, 7).Add(13 * time.Hour), end: now.AddDate(0, 0, 7).Add(14 * time.Hour)},
	} {
		if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: tc.desc, StartedAt: tc.start, EndedAt: tc.end}); err != nil {
			t.Fatalf("CreateManualEntry(%s) error = %v", tc.desc, err)
		}
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 28})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	app = updated.(AppModel)

	if !app.displayedDay().Equal(now) {
		t.Fatalf("initial dashboard day = %s, want %s", app.displayedDay().Format("2006-01-02"), now.Format("2006-01-02"))
	}
	if !strings.Contains(stripANSI(app.View()), "Today work") {
		t.Fatalf("dashboard view missing today stream: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(now.AddDate(0, 0, 1)) {
		t.Fatalf("dashboard day after j = %s, want %s", app.displayedDay().Format("2006-01-02"), now.AddDate(0, 0, 1).Format("2006-01-02"))
	}
	view := stripANSI(app.View())
	if !strings.Contains(view, "Tomorrow work") {
		t.Fatalf("dashboard view missing moved stream: %q", view)
	}
	if strings.Contains(view, "Today work") {
		t.Fatalf("dashboard view still shows previous day stream: %q", view)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(now) {
		t.Fatalf("dashboard day after k = %s, want %s", app.displayedDay().Format("2006-01-02"), now.Format("2006-01-02"))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(now.AddDate(0, 0, 7)) {
		t.Fatalf("dashboard day after l = %s, want %s", app.displayedDay().Format("2006-01-02"), now.AddDate(0, 0, 7).Format("2006-01-02"))
	}
	view = stripANSI(app.View())
	if !strings.Contains(view, "Next week work") {
		t.Fatalf("dashboard view missing horizontal move stream: %q", view)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	app = updated.(AppModel)
	if !app.displayedDay().Equal(now) {
		t.Fatalf("dashboard day after h = %s, want %s", app.displayedDay().Format("2006-01-02"), now.Format("2006-01-02"))
	}
}

func TestDashboardEnterOpensSelectedDayInDayView(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	base := dayStart(time.Now().In(time.Local))
	target := base.AddDate(0, 0, -3)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Chosen day", StartedAt: target.Add(9 * time.Hour), EndedAt: target.Add(10 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 28})
	app := updated.(AppModel)
	app.dayDate = target
	app.loadDashboardForDay(target)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	app = updated.(AppModel)

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeTimeline {
		t.Fatalf("mode after enter = %q, want timeline", app.mode)
	}
	if app.timelineView != timelineViewDay {
		t.Fatalf("timelineView after enter = %q, want day", app.timelineView)
	}
	if !app.displayedDay().Equal(target) {
		t.Fatalf("displayedDay after enter = %s, want %s", app.displayedDay().Format("2006-01-02"), target.Format("2006-01-02"))
	}
	if !strings.Contains(stripANSI(app.View()), "Chosen day") {
		t.Fatalf("day view missing selected day entry: %q", stripANSI(app.View()))
	}
}

func TestDashboardActivityPaneCanScroll(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := dayStart(time.Now().In(time.Local))
	for i := 0; i < 10; i++ {
		start := now.Add(time.Duration(8+i) * time.Hour)
		if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: fmt.Sprintf("Entry %02d", i), StartedAt: start, EndedAt: start.Add(30 * time.Minute)}); err != nil {
			t.Fatalf("CreateManualEntry(%d) error = %v", i, err)
		}
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 22})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	app = updated.(AppModel)
	if app.dashboardViewport.TotalLineCount() <= app.dashboardViewport.Height {
		t.Fatalf("dashboard viewport not scrollable: total=%d height=%d", app.dashboardViewport.TotalLineCount(), app.dashboardViewport.Height)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	app = updated.(AppModel)
	if app.dashboardViewport.YOffset == 0 {
		t.Fatalf("dashboard viewport did not scroll: %#v", app.dashboardViewport)
	}
	if app.dashboardViewport.TotalLineCount() <= app.dashboardViewport.Height {
		t.Fatalf("dashboard viewport unexpectedly lost overflow after scroll: total=%d height=%d", app.dashboardViewport.TotalLineCount(), app.dashboardViewport.Height)
	}
}

func TestDashboardWideLayoutStaysWithinViewportAndShowsBothColumns(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "USD"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := dayStart(time.Now().In(time.Local))
	for i := 0; i < 8; i++ {
		start := now.Add(time.Duration(8+i) * time.Hour)
		if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "hrs", Description: fmt.Sprintf("Dashboard entry %02d", i), StartedAt: start, EndedAt: start.Add(45 * time.Minute)}); err != nil {
			t.Fatalf("CreateManualEntry(%d) error = %v", i, err)
		}
	}
	if err := store.EnsureProjectDefaultTimeOffTypes(ctx, project.ID); err != nil {
		t.Fatalf("EnsureProjectDefaultTimeOffTypes() error = %v", err)
	}
	types, err := store.ListTimeOffTypesByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListTimeOffTypesByProject() error = %v", err)
	}
	var vacationID string
	for _, item := range types {
		if item.Name == "Vacation" {
			vacationID = item.ID
			break
		}
	}
	if vacationID == "" {
		t.Fatal("vacation type not found")
	}
	if _, err := store.UpsertTimeOffDay(ctx, project.ID, vacationID, now.AddDate(0, 0, 5).Format("2006-01-02")); err != nil {
		t.Fatalf("UpsertTimeOffDay() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 180, Height: 40})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	app = updated.(AppModel)
	out := stripANSI(app.View())
	for _, want := range []string{"Year heatmap", "Daily activity", "Year progress", "Planned time off", "Dashboard entry 00"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dashboard missing %q:\n%s", want, out)
		}
	}
	assertViewLinesFitWidth(t, out, 180)
	if blank := longestBlankRun(out); blank > 6 {
		t.Fatalf("dashboard contains suspicious blank run of %d lines:\n%s", blank, out)
	}
	if nonSpaceAfter(out, "Daily activity") < 120 {
		t.Fatalf("dashboard body too empty after daily activity:\n%s", out)
	}
}

func TestDashboardLongAgentActivityStillShowsVisibleBody(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := dayStart(time.Now().In(time.Local))
	for i, desc := range []string{
		"migration follow up for multiple datasets",
		"michal interview",
		"multiple dataset db restrictions",
		"prepare for Marc interview",
		"nick & marc sync",
	} {
		start := now.Add(time.Duration(10+i) * time.Hour)
		if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: desc, StartedAt: start, EndedAt: start.Add(time.Hour)}); err != nil {
			t.Fatalf("CreateManualEntry(%d) error = %v", i, err)
		}
	}
	slots := make([]model.ActivitySlot, 0, 20)
	for i := 0; i < 20; i++ {
		slots = append(slots, model.ActivitySlot{
			SlotTime:   now.Add(time.Duration(8+i) * 15 * time.Minute).UTC(),
			Operator:   "claude-code",
			MsgCount:   40 + i,
			FirstText:  fmt.Sprintf("very long prompt %02d inside the review flow with enough text to stress wrapping and viewport rendering", i),
			TokenInput: 1000 + i,
		})
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 220, Height: 45})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	app = updated.(AppModel)
	out := stripANSI(app.View())
	for _, want := range []string{"Daily activity", "Year progress", "Planned time off", "michal interview", "multiple dataset db restrictions"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dashboard missing %q:\n%s", want, out)
		}
	}
	assertViewLinesFitWidth(t, out, 220)
	if nonSpaceAfter(out, "Daily activity") < 200 {
		t.Fatalf("dashboard activity area too empty under heavy agent data:\n%s", out)
	}
}

func TestDashboardStatusBarSpansFullWidth(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := dayStart(time.Now().In(time.Local))
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "hrs", Description: "Status bar check", StartedAt: now.Add(9 * time.Hour), EndedAt: now.Add(10 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	app = updated.(AppModel)
	lines := strings.Split(strings.TrimRight(stripANSI(app.View()), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("dashboard view empty")
	}
	last := lines[len(lines)-1]
	if lipgloss.Width(last) != 120 {
		t.Fatalf("status bar width = %d, want 120\n%q", lipgloss.Width(last), last)
	}
}

func TestDashboardShowsTimeOffAllowanceSummaries(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	project, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "USD"})
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
	if _, err := store.UpsertTimeOffAllowance(ctx, project.ID, vacation.ID, time.Now().Year(), 20); err != nil {
		t.Fatalf("UpsertTimeOffAllowance() error = %v", err)
	}
	if _, err := store.UpsertTimeOffDay(ctx, project.ID, vacation.ID, time.Now().Format("2006-01-02")); err != nil {
		t.Fatalf("UpsertTimeOffDay() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	app = updated.(AppModel)
	out := stripANSI(app.View())
	if !strings.Contains(out, "Vacation @ hrs 1/20 used") {
		t.Fatalf("dashboard allowance summary missing: %q", out)
	}
}

func TestReportViewCanSwitchRangePresets(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", HourlyRate: 15000, Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	now := time.Now().In(time.Local)
	weekStart, weekEnd := reportWeekRange(now)
	monthStart, monthEnd := reportMonthRange(now)
	yearStart, yearEnd := reportYearRange(now)

	weekEntryStart := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
	monthOnlyStart := firstDayOutsideRange(monthStart, monthEnd, weekStart, weekEnd)
	yearOnlyStart := firstDayOutsideRange(yearStart, yearEnd, monthStart, monthEnd)

	for _, tc := range []struct {
		start       time.Time
		description string
	}{
		{start: weekEntryStart, description: "week work"},
		{start: monthOnlyStart, description: "month work"},
		{start: yearOnlyStart, description: "year work"},
	} {
		if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
			ProjectIdent: "elaiia",
			Description:  tc.description,
			StartedAt:    tc.start,
			EndedAt:      tc.start.Add(time.Hour),
		}); err != nil {
			t.Fatalf("CreateManualEntry(%s) error = %v", tc.description, err)
		}
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	if !strings.Contains(stripANSI(app.View()), "Total hours: 1.0") {
		t.Fatalf("week report total mismatch: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app = updated.(AppModel)
	if !strings.Contains(stripANSI(app.View()), "Total hours: 2.0") {
		t.Fatalf("month report total mismatch: %q", stripANSI(app.View()))
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	app = updated.(AppModel)
	if !strings.Contains(stripANSI(app.View()), "Total hours: 3.0") {
		t.Fatalf("year report total mismatch: %q", stripANSI(app.View()))
	}
}

func TestReportViewCanSelectProjects(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	for _, project := range []db.ProjectCreateInput{
		{Name: "Alpha", Code: "alpha", Currency: "CHF"},
		{Name: "Beta", Code: "beta", Currency: "USD"},
	} {
		if _, err := store.CreateProject(ctx, project); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", project.Name, err)
		}
	}
	now := time.Now().In(time.Local)
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "alpha",
		Description:  "Alpha work",
		StartedAt:    start,
		EndedAt:      start.Add(2 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateManualEntry(alpha) error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "beta",
		Description:  "Beta work",
		StartedAt:    start.Add(3 * time.Hour),
		EndedAt:      start.Add(4 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateManualEntry(beta) error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	view := stripANSI(app.View())
	if !strings.Contains(view, "Selected project: Alpha") {
		t.Fatalf("default selected project missing: %q", view)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = updated.(AppModel)
	view = stripANSI(app.View())
	if !strings.Contains(view, "Selected project: Beta") {
		t.Fatalf("selection did not move to beta: %q", view)
	}
}

func TestReportViewShowsRelativeProjectBars(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	for _, project := range []db.ProjectCreateInput{
		{Name: "Alpha", Code: "alpha", Currency: "CHF"},
		{Name: "Beta", Code: "beta", Currency: "USD"},
	} {
		if _, err := store.CreateProject(ctx, project); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", project.Name, err)
		}
	}
	now := time.Now().In(time.Local)
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "alpha",
		Description:  "Alpha work",
		StartedAt:    start,
		EndedAt:      start.Add(4 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateManualEntry(alpha) error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "beta",
		Description:  "Beta work",
		StartedAt:    start.Add(5 * time.Hour),
		EndedAt:      start.Add(6 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateManualEntry(beta) error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	view := stripANSI(app.View())
	if !strings.Contains(view, "Alpha 4.0h") || !strings.Contains(view, "Beta 1.0h") {
		t.Fatalf("report view missing project totals: %q", view)
	}
	if !strings.Contains(view, "████") || !strings.Contains(view, "█") {
		t.Fatalf("report view missing relative bars: %q", view)
	}
}

func TestReportViewUsesCompactTwoColumnLayout(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	for _, project := range []db.ProjectCreateInput{
		{Name: "Alpha", Code: "alpha", Currency: "CHF"},
		{Name: "Beta", Code: "beta", Currency: "USD"},
	} {
		if _, err := store.CreateProject(ctx, project); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", project.Name, err)
		}
	}
	now := time.Now().In(time.Local)
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "alpha",
		Description:  "Alpha work",
		StartedAt:    start,
		EndedAt:      start.Add(4 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateManualEntry(alpha) error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "beta",
		Description:  "Beta work",
		StartedAt:    start.Add(5 * time.Hour),
		EndedAt:      start.Add(6 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateManualEntry(beta) error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.width = 100

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	view := stripANSI(app.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")

	foundCompactRow := false
	for _, line := range lines {
		if strings.Contains(line, "Summary") && strings.Contains(line, "By project") {
			foundCompactRow = true
			break
		}
	}
	if !foundCompactRow {
		t.Fatalf("report view did not render summary and project sections side-by-side: %q", view)
	}
	if !strings.Contains(view, "Selected project: Alpha") {
		t.Fatalf("report detail missing in compact layout: %q", view)
	}
}

func TestReportViewPinsStatusBarToBottom(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	start := time.Date(2026, 4, 18, 9, 0, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "alpha", Description: "Work", StartedAt: start, EndedAt: start.Add(time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.width = 100
	model.height = 24

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	app.width = 100
	app.height = 24

	view := stripANSI(app.View())
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if got := lipgloss.Height(app.View()); got != app.height {
		t.Fatalf("report view height = %d, want %d\n%s", got, app.height, view)
	}
	if !strings.Contains(lines[len(lines)-1], "report week") {
		t.Fatalf("report status bar not at bottom: %q", lines[len(lines)-1])
	}
	if strings.TrimSpace(lines[len(lines)-2]) != "" {
		t.Fatalf("expected spacer line above bottom status bar, got %q", lines[len(lines)-2])
	}
}

func TestReportViewShowsDailyBreakdownBars(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := time.Now().In(time.Local)
	weekStart, _ := reportWeekRange(now)
	dayOne := time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 9, 0, 0, 0, time.Local)
	dayTwo := dayOne.AddDate(0, 0, 1)
	for _, tc := range []struct {
		start time.Time
		end   time.Time
	}{
		{start: dayOne, end: dayOne.Add(4 * time.Hour)},
		{start: dayTwo, end: dayTwo.Add(2 * time.Hour)},
	} {
		if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
			ProjectIdent: "alpha",
			Description:  "Daily work",
			StartedAt:    tc.start,
			EndedAt:      tc.end,
		}); err != nil {
			t.Fatalf("CreateManualEntry() error = %v", err)
		}
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.width = 100

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	view := stripANSI(app.View())
	if !strings.Contains(view, "By day") {
		t.Fatalf("report view missing daily section: %q", view)
	}
	if !strings.Contains(view, "4.0h") || !strings.Contains(view, "2.0h") {
		t.Fatalf("report view missing daily totals: %q", view)
	}
	if !strings.Contains(view, "████") {
		t.Fatalf("report view missing daily bars: %q", view)
	}
}

func TestReportViewShowsEarnedEstimate(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "CHF", HourlyRate: 15000}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := time.Now().In(time.Local)
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "alpha",
		Description:  "Billable work",
		StartedAt:    start,
		EndedAt:      start.Add(2 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.width = 100

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	view := stripANSI(app.View())
	if !strings.Contains(view, "Earned: 300.00 CHF") {
		t.Fatalf("report view missing earned estimate: %q", view)
	}
}

func TestReportViewShowsSummaryEarnedTotal(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "CHF", HourlyRate: 15000}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	now := time.Now().In(time.Local)
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "alpha",
		Description:  "Billable work",
		StartedAt:    start,
		EndedAt:      start.Add(2 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.width = 100

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	view := stripANSI(app.View())
	if !strings.Contains(view, "Earned total: 300.00 CHF") {
		t.Fatalf("report summary missing earned total: %q", view)
	}
}

func TestReportViewShowsProjectSharePercentages(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	for _, project := range []db.ProjectCreateInput{
		{Name: "Alpha", Code: "alpha", Currency: "CHF"},
		{Name: "Beta", Code: "beta", Currency: "USD"},
	} {
		if _, err := store.CreateProject(ctx, project); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", project.Name, err)
		}
	}
	now := time.Now().In(time.Local)
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "alpha", Description: "Alpha work", StartedAt: start, EndedAt: start.Add(4 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry(alpha) error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "beta", Description: "Beta work", StartedAt: start.Add(5 * time.Hour), EndedAt: start.Add(6 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry(beta) error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.width = 100

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	view := stripANSI(app.View())
	if !strings.Contains(view, "80%") || !strings.Contains(view, "20%") {
		t.Fatalf("report view missing project share percentages: %q", view)
	}
}

func TestReportViewRefreshesAfterSyncDone(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	weekStart, _ := reportWeekRange(time.Now().In(time.Local))
	start := weekStart.Add(9 * time.Hour)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "alpha", Description: "First", StartedAt: start, EndedAt: start.Add(time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry(first) error = %v", err)
	}

	model, err := NewAppModelWithSync(ctx, store, func() error { return nil })
	if err != nil {
		t.Fatalf("NewAppModelWithSync() error = %v", err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	if !strings.Contains(stripANSI(app.View()), "Total hours: 1.0") {
		t.Fatalf("initial report total mismatch: %q", stripANSI(app.View()))
	}

	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "alpha", Description: "Second", StartedAt: start.Add(2 * time.Hour), EndedAt: start.Add(4 * time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry(second) error = %v", err)
	}

	updated, _ = app.Update(syncDoneMsg{err: nil})
	app = updated.(AppModel)
	if !strings.Contains(stripANSI(app.View()), "Total hours: 3.0") {
		t.Fatalf("report total was not refreshed after sync: %q", stripANSI(app.View()))
	}
}

func TestReportViewDoesNotMutateHiddenDayState(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	entryStart := time.Date(2026, 4, 18, 9, 0, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "alpha", Description: "Day work", StartedAt: entryStart, EndedAt: entryStart.Add(time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	model.InitializeTodayTimelineView()
	before := model.displayedDay()

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)

	after := app.displayedDay()
	if !after.Equal(before) {
		t.Fatalf("displayedDay changed in report mode: before=%s after=%s", before.Format("2006-01-02"), after.Format("2006-01-02"))
	}
}

func TestReportViewQuitsOnQ(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Alpha", Code: "alpha", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	start := time.Date(2026, 4, 18, 9, 0, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "alpha", Description: "Work", StartedAt: start, EndedAt: start.Add(time.Hour)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	model, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	app := updated.(AppModel)

	updated, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	app = updated.(AppModel)
	if !app.quitting {
		t.Fatal("quitting = false after q in report mode, want true")
	}
	if cmd == nil {
		t.Fatal("cmd = nil after q in report mode, want quit command")
	}
}

func firstDayOutsideRange(searchStart, searchEnd, excludedStart, excludedEnd time.Time) time.Time {
	for day := searchStart; day.Before(searchEnd); day = day.AddDate(0, 0, 1) {
		if day.Before(excludedStart) || !day.Before(excludedEnd) {
			return time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, time.Local)
		}
	}
	return time.Date(searchStart.Year(), searchStart.Month(), searchStart.Day(), 9, 0, 0, 0, time.Local)
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

func TestDeleteWordInEntryEditDialog(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Hello there", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true})
	app = updated.(AppModel)
	if app.entryInput != "Hello " {
		t.Fatalf("after alt+backspace entryInput = %q, want %q", app.entryInput, "Hello ")
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	app = updated.(AppModel)
	if app.entryInput != "" {
		t.Fatalf("after ctrl+w entryInput = %q, want empty", app.entryInput)
	}
}

func TestWordNavigationInEntryEditDialog(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Hello brave world", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}, Alt: true})
	app = updated.(AppModel)
	if app.entryInputCursor != len([]rune("Hello brave ")) {
		t.Fatalf("entryInputCursor = %d, want %d", app.entryInputCursor, len([]rune("Hello brave ")))
	}
	for _, r := range "new " {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	if app.entryInput != "Hello brave new world" {
		t.Fatalf("entryInput = %q, want %q", app.entryInput, "Hello brave new world")
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true})
	app = updated.(AppModel)
	if app.entryInputCursor != len([]rune("Hello brave new world")) {
		t.Fatalf("entryInputCursor = %d, want end", app.entryInputCursor)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	app = updated.(AppModel)
	if app.entryInput != "Hello brave new world!" {
		t.Fatalf("entryInput = %q, want %q", app.entryInput, "Hello brave new world!")
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

func TestDeleteWordInGapDialog(t *testing.T) {
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

	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	for _, r := range "abc def" {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true})
	app = updated.(AppModel)
	if app.gapInput != "abc " {
		t.Fatalf("after alt+backspace gapInput = %q, want %q", app.gapInput, "abc ")
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	app = updated.(AppModel)
	if app.gapInput != "" {
		t.Fatalf("after ctrl+w gapInput = %q, want empty", app.gapInput)
	}
}

func TestWordNavigationInGapDialog(t *testing.T) {
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

	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	for _, r := range "Alpha beta gamma" {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
	app = updated.(AppModel)
	if app.gapInputCursor != len([]rune("Alpha beta ")) {
		t.Fatalf("gapInputCursor = %d, want %d", app.gapInputCursor, len([]rune("Alpha beta ")))
	}
	for _, r := range "new " {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = updated.(AppModel)
	}
	if app.gapInput != "Alpha beta new gamma" {
		t.Fatalf("gapInput = %q, want %q", app.gapInput, "Alpha beta new gamma")
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
	if !strings.Contains(lines[0], "Enter/c") {
		t.Fatalf("slot action lines = %v, want Enter/c hint", lines)
	}

	// Gap mode
	app.dayFocusKind = "gap"
	lines = actionInspectorLines(app)
	if len(lines) == 0 || !strings.Contains(lines[0], "manual entry") {
		t.Fatalf("gap action lines = %v, want manual entry hint", lines)
	}
	if !strings.Contains(lines[0], "Enter/c") {
		t.Fatalf("gap action lines = %v, want Enter/c hint", lines)
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

func TestCOpensGapEntryDialogFromDaySlot(t *testing.T) {
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

	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	app = updated.(AppModel)
	if app.mode != modeGapEntry {
		t.Fatalf("mode = %q, want gap-entry", app.mode)
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

	// Press x to open delete dialog
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
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

func assertViewLinesFitWidth(t *testing.T, view string, width int) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if lipgloss.Width(line) > width {
			t.Fatalf("line width = %d, want <= %d\n%s", lipgloss.Width(line), width, line)
		}
	}
}

func longestBlankRun(view string) int {
	maxRun := 0
	run := 0
	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			run++
			if run > maxRun {
				maxRun = run
			}
			continue
		}
		run = 0
	}
	return maxRun
}

func nonSpaceAfter(view string, marker string) int {
	idx := strings.Index(view, marker)
	if idx < 0 {
		return 0
	}
	count := 0
	for _, r := range view[idx+len(marker):] {
		if r != ' ' && r != '\n' && r != '\t' && r != '\r' {
			count++
		}
	}
	return count
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

	// Navigate backward with k: C -> B -> A, then stays on A
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	app = updated.(AppModel)
	if app.dayFocusKind != "entry" {
		t.Fatalf("after k: focus = %q, want entry", app.dayFocusKind)
	}
	if descriptionOrID(app.entries[app.cursor]) != "B" {
		t.Fatalf("after 1st k: entry = %q, want B", descriptionOrID(app.entries[app.cursor]))
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	app = updated.(AppModel)
	if descriptionOrID(app.entries[app.cursor]) != "A" {
		t.Fatalf("after 2nd k: entry = %q, want A", descriptionOrID(app.entries[app.cursor]))
	}
	// k at first entry stays on A
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	app = updated.(AppModel)
	if descriptionOrID(app.entries[app.cursor]) != "A" {
		t.Fatalf("after 3rd k: entry = %q, want A (should stay)", descriptionOrID(app.entries[app.cursor]))
	}

	// Navigate forward with j: A -> B -> C, then stays on C
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app = updated.(AppModel)
	if descriptionOrID(app.entries[app.cursor]) != "B" {
		t.Fatalf("after 1st j: entry = %q, want B", descriptionOrID(app.entries[app.cursor]))
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app = updated.(AppModel)
	if descriptionOrID(app.entries[app.cursor]) != "C" {
		t.Fatalf("after 2nd j: entry = %q, want C", descriptionOrID(app.entries[app.cursor]))
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

	// Move down several times to expand range
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
	// Range should start at 09:00 and extend past it
	if clock(rng.start) != "09:00" {
		t.Fatalf("range start = %s, want 09:00", clock(rng.start))
	}
	if !rng.end.After(rng.start.Add(30 * time.Minute)) {
		t.Fatalf("range too short: %s-%s", clock(rng.start), clock(rng.end))
	}

	// Press enter to open gap dialog — should have the marked range
	expectedEnd := clock(rng.end)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeGapEntry {
		t.Fatalf("mode = %q, want gap-entry", app.mode)
	}
	if app.gapStartInput != "09:00" {
		t.Fatalf("gapStartInput = %q, want 09:00", app.gapStartInput)
	}
	if app.gapEndInput != expectedEnd {
		t.Fatalf("gapEndInput = %q, want %s", app.gapEndInput, expectedEnd)
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

	// Verify entry was created with correct range (start=09:00, end matches marked range)
	entries, _ := store.ListEntries(ctx)
	found := false
	for _, e := range entries {
		if e.Description != nil && *e.Description == "Marked entry" {
			r := formatRange(e.StartedAt, e.EndedAt)
			if clock(e.StartedAt) != "09:00" {
				t.Fatalf("created entry start = %s, want 09:00", clock(e.StartedAt))
			}
			if clock(*e.EndedAt) != expectedEnd {
				t.Fatalf("created entry range = %s, want 09:00-%s", r, expectedEnd)
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
			if clock(e.StartedAt) != "09:00" {
				t.Fatalf("edited entry start changed: %s, want 09:00", clock(e.StartedAt))
			}
			if clock(*e.EndedAt) != expectedEnd {
				t.Fatalf("edited entry end changed: %s, want %s", clock(*e.EndedAt), expectedEnd)
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

func TestShiftUpDownMovesSlotOneHourLegacy(t *testing.T) {
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

	// shift+down: cursor moves from 09:00 to 10:00
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	app = updated.(AppModel)
	if clock(app.daySlotStart) != "10:00" {
		t.Fatalf("after shift+down: slot = %s, want 10:00", clock(app.daySlotStart))
	}

	// Another shift+down: 10:00 to 11:00
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	app = updated.(AppModel)
	if clock(app.daySlotStart) != "11:00" {
		t.Fatalf("after 2nd shift+down: slot = %s, want 11:00", clock(app.daySlotStart))
	}

	// shift+up: 11:00 to 10:00
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	app = updated.(AppModel)
	if clock(app.daySlotStart) != "10:00" {
		t.Fatalf("after shift+up: slot = %s, want 10:00", clock(app.daySlotStart))
	}

	// esc should clear any marks
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app = updated.(AppModel)
	if !app.slotMarkStart.IsZero() {
		t.Fatal("slotMarkStart not cleared by esc")
	}
	if app.slotMarkSpan != 0 {
		t.Fatalf("slotMarkSpan = %v, want 0", app.slotMarkSpan)
	}
	rng := app.selectedCreateRange()
	if rng == nil {
		t.Fatal("selectedCreateRange() nil after esc")
	}
	dur := rng.end.Sub(rng.start)
	if dur != 15*time.Minute {
		t.Fatalf("range duration after esc = %v, want 15m", dur)
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
	app.dayDate = dayStart(time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	app = updated.(AppModel)

	// Focus on earliest entry ("agent session") to navigate forward through all
	indices := app.dayEntryIndices(app.displayedDay().Format("2006-01-02"))
	if len(indices) > 0 {
		app.cursor = indices[0]
		app.dayFocusKind = "entry"
	}

	// Navigate forward with j — should visit each entry, always staying on "entry" focus
	seen := map[string]bool{descriptionOrID(app.entries[app.cursor]): true}
	for i := 0; i < 5; i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		app = updated.(AppModel)
		if app.dayFocusKind != "entry" {
			t.Fatalf("j press %d: focus = %q, want entry", i+1, app.dayFocusKind)
		}
		seen[descriptionOrID(app.entries[app.cursor])] = true
	}
	// should have visited all 3 entries
	for _, name := range []string{"manual work", "follow up", "afternoon"} {
		if !seen[name] {
			t.Errorf("never visited entry %q, seen = %v", name, seen)
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

	// j at last entry should stay on entry (no gap navigation)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app = updated.(AppModel)
	if app.dayFocusKind != "entry" {
		t.Fatalf("after j at last entry: focus = %q, want entry (should stay)", app.dayFocusKind)
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

	// press x to open confirm dialog
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
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

	// press x then y to confirm delete
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
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

func TestTimelineDOpensDashboardInsteadOfDelete(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "USD"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "p",
		Description:  "Keep me",
		StartedAt:    time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = updated.(AppModel)
	if app.mode != modeDashboard {
		t.Fatalf("mode = %q, want %q", app.mode, modeDashboard)
	}
	if app.confirmDeleteID != "" {
		t.Fatalf("confirmDeleteID = %q, want empty", app.confirmDeleteID)
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
	if got := lipgloss.Height(app.View()); got != app.height {
		t.Fatalf("day view height = %d, want %d\n%s", got, app.height, stripANSI(app.View()))
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

func TestDayViewInspectorShowsOverlappingActivityForEntry(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	entryStart := time.Date(2026, 4, 7, 8, 45, 0, 0, time.Local)
	entryEnd := time.Date(2026, 4, 7, 9, 30, 0, 0, time.Local)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "improve TUI experience",
		StartedAt:    entryStart,
		EndedAt:      entryEnd,
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	if err := store.UpsertActivitySlots(ctx, []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 8, 45, 0, 0, time.Local).UTC(), Operator: "claude-code", MsgCount: 21, Cwd: "/Users/akoskm/Projects/hrs", GitBranch: "main", UserTexts: []string{"please create a README.md file with setup instructions"}},
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local).UTC(), Operator: "claude-code", MsgCount: 18, Cwd: "/Users/akoskm/Projects/hrs", GitBranch: "main", UserTexts: []string{"review recent code changes and improvement areas"}},
	}); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(entryStart)
	m.dayFocusKind = "entry"
	m.cursor = 0
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 30})
	app := updated.(AppModel)
	view := stripANSI(app.View())

	checks := []string{
		"improve TUI experience",
		"Agent activity: 2 slots",
		"claude-code (21 msgs)",
		"please create a README.md file with setup instructions",
		"review recent code changes and improvement areas",
	}
	for _, want := range checks {
		if !strings.Contains(view, want) {
			t.Fatalf("inspector missing %q, got:\n%s", want, view)
		}
	}
}

func TestDayViewInspectorShowsMarkedRangeActivity(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	day := time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local)
	if err := store.UpsertActivitySlots(ctx, []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local).UTC(), Operator: "claude-code", MsgCount: 10, UserTexts: []string{"first prompt"}},
		{SlotTime: time.Date(2026, 4, 7, 8, 15, 0, 0, time.Local).UTC(), Operator: "claude-code", MsgCount: 12, UserTexts: []string{"second prompt"}},
	}); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(day)
	m.dayFocusKind = "slot"
	m.daySlotStart = time.Date(2026, 4, 7, 8, 15, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	m.slotMarkStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.slotMarkSpan = 15 * time.Minute
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 30})
	app := updated.(AppModel)
	view := stripANSI(app.View())

	checks := []string{
		"Activity: 08:00-08:30",
		"Agent activity: 2 slots",
		"first prompt",
		"second prompt",
	}
	for _, want := range checks {
		if !strings.Contains(view, want) {
			t.Fatalf("inspector missing %q, got:\n%s", want, view)
		}
	}
}

func TestDayViewFiltersLowSignalActivitySlots(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	day := time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local)
	if err := store.UpsertActivitySlots(ctx, []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local).UTC(), Operator: "hidden-agent", MsgCount: 5},
		{SlotTime: time.Date(2026, 4, 7, 8, 15, 0, 0, time.Local).UTC(), Operator: "visible-agent", MsgCount: 7, FirstText: "real work", UserTexts: []string{"real work"}},
	}); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(day)
	m.dayFocusKind = "slot"
	m.daySlotStart = time.Date(2026, 4, 7, 8, 15, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	m.slotMarkStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.slotMarkSpan = 15 * time.Minute
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 30})
	app := updated.(AppModel)
	view := stripANSI(app.View())

	checks := []string{
		"1 active slots",
		"Agent activity: 1 slot",
		"real work",
	}
	for _, want := range checks {
		if !strings.Contains(view, want) {
			t.Fatalf("day view missing %q, got:\n%s", want, view)
		}
	}
	if strings.Contains(view, "hidden-agent") {
		t.Fatalf("day view should hide low-signal slot, got:\n%s", view)
	}
}

func TestRenderInspectorBodyClipsToViewportHeight(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	day := time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local)
	if err := store.UpsertActivitySlots(ctx, []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local).UTC(), Operator: "claude-code", MsgCount: 10, UserTexts: []string{"prompt 1"}},
		{SlotTime: time.Date(2026, 4, 7, 8, 15, 0, 0, time.Local).UTC(), Operator: "claude-code", MsgCount: 12, UserTexts: []string{"prompt 2"}},
		{SlotTime: time.Date(2026, 4, 7, 8, 30, 0, 0, time.Local).UTC(), Operator: "claude-code", MsgCount: 14, UserTexts: []string{"prompt 3"}},
	}); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(day)
	m.dayFocusKind = "slot"
	m.daySlotStart = time.Date(2026, 4, 7, 8, 30, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	m.slotMarkStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.slotMarkSpan = 15 * time.Minute
	m.loadActivitySlots()

	body := renderInspectorBody(m, 80, 8)
	lines := strings.Split(body, "\n")
	if len(lines) != 8 {
		t.Fatalf("inspector body line count = %d, want 8", len(lines))
	}
	if !strings.Contains(body, "prompt 1") {
		t.Fatalf("inspector body missing first prompt, got:\n%s", body)
	}
	if strings.Contains(body, "prompt 3") {
		t.Fatalf("inspector body should be clipped, got:\n%s", body)
	}
}

func TestDayViewInspectorScrollsWithoutStretchingLayout(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	day := time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local)
	slots := make([]model.ActivitySlot, 0, 6)
	for i := 0; i < 6; i++ {
		slots = append(slots, model.ActivitySlot{
			SlotTime:  time.Date(2026, 4, 7, 8, i*15, 0, 0, time.Local).UTC(),
			Operator:  "claude-code",
			MsgCount:  10 + i,
			UserTexts: []string{"prompt " + strconv.Itoa(i+1)},
		})
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(day)
	m.dayFocusKind = "slot"
	m.daySlotStart = time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	m.slotMarkStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.slotMarkSpan = 15 * time.Minute
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 18})
	app := updated.(AppModel)

	view := stripANSI(app.View())
	if !strings.Contains(view, "prompt 1") {
		t.Fatalf("initial inspector missing early prompt, got:\n%s", view)
	}
	if strings.Contains(view, "prompt 6") {
		t.Fatalf("initial inspector should be clipped before prompt 6, got:\n%s", view)
	}
	targetHeight := dayPaneHeight(app.height)
	pane := renderInspectorPane(app, app.styles, max(20, app.width/2), targetHeight)
	if lipgloss.Height(pane) != targetHeight {
		t.Fatalf("inspector pane height = %d, want %d", lipgloss.Height(pane), targetHeight)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	app = updated.(AppModel)
	if app.inspectorViewport.YOffset == 0 {
		t.Fatal("inspector viewport did not scroll after pgdown")
	}
	view = stripANSI(app.View())
	if !strings.Contains(view, "prompt 4") {
		t.Fatalf("scrolled inspector missing later prompt, got:\n%s", view)
	}
	if strings.Contains(view, "prompt 1") {
		t.Fatalf("scrolled inspector still shows first prompt, got:\n%s", view)
	}
	pane = renderInspectorPane(app, app.styles, max(20, app.width/2), targetHeight)
	if lipgloss.Height(pane) != targetHeight {
		t.Fatalf("scrolled inspector pane height = %d, want %d", lipgloss.Height(pane), targetHeight)
	}
}

func TestDayViewMouseWheelScrollsInspectorWhenHovered(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	day := time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local)
	slots := make([]model.ActivitySlot, 0, 6)
	for i := 0; i < 6; i++ {
		slots = append(slots, model.ActivitySlot{
			SlotTime:  time.Date(2026, 4, 7, 8, i*15, 0, 0, time.Local).UTC(),
			Operator:  "claude-code",
			MsgCount:  10 + i,
			UserTexts: []string{"prompt " + strconv.Itoa(i+1)},
		})
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(day)
	m.dayFocusKind = "slot"
	m.daySlotStart = time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	m.slotMarkStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.slotMarkSpan = 15 * time.Minute
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 18})
	app := updated.(AppModel)
	initialView := stripANSI(app.View())
	if !strings.Contains(initialView, "prompt 1") {
		t.Fatalf("initial inspector missing first prompt, got:\n%s", initialView)
	}
	if strings.Contains(initialView, "prompt 6") {
		t.Fatalf("initial inspector should be clipped before prompt 6, got:\n%s", initialView)
	}

	startWindow := app.dayWindowStart
	inspectorX := app.width - dayInspectorWidth(app.width) + 1
	updated, _ = app.Update(tea.MouseMsg{X: inspectorX, Button: tea.MouseButtonWheelDown})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.MouseMsg{X: inspectorX, Button: tea.MouseButtonWheelDown})
	app = updated.(AppModel)
	if !app.dayWindowStart.Equal(startWindow) {
		t.Fatalf("dayWindowStart changed from %s to %s while scrolling inspector", clock(startWindow), clock(app.dayWindowStart))
	}

	view := stripANSI(app.View())
	if !strings.Contains(view, "prompt 4") {
		t.Fatalf("wheel-scrolled inspector missing later prompt, got:\n%s", view)
	}
	if strings.Contains(view, "prompt 1") {
		t.Fatalf("wheel-scrolled inspector still shows first prompt, got:\n%s", view)
	}
}

func TestDayViewInspectorResetsToTopWhenTabChanges(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	day := time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local)
	slots := make([]model.ActivitySlot, 0, 6)
	for i := 0; i < 6; i++ {
		slots = append(slots, model.ActivitySlot{
			SlotTime:  time.Date(2026, 4, 7, 8, i*15, 0, 0, time.Local).UTC(),
			Operator:  "claude-code",
			MsgCount:  10 + i,
			UserTexts: []string{"prompt " + strconv.Itoa(i+1)},
		})
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(day)
	m.dayFocusKind = "slot"
	m.daySlotStart = time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	m.slotMarkStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.slotMarkSpan = 15 * time.Minute
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 18})
	app := updated.(AppModel)

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	app = updated.(AppModel)
	view := stripANSI(app.View())
	if !strings.Contains(view, "prompt 4") {
		t.Fatalf("scrolled overview missing later prompt, got:\n%s", view)
	}
	if strings.Contains(view, "prompt 1") {
		t.Fatalf("scrolled overview still shows first prompt, got:\n%s", view)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	view = stripANSI(app.View())
	if !strings.Contains(view, "Enter/c: create entry or edit overlap") {
		t.Fatalf("actions tab missing top actions, got:\n%s", view)
	}

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	view = stripANSI(app.View())
	if !strings.Contains(view, "prompt 1") {
		t.Fatalf("overview did not reset to top after tab cycle, got:\n%s", view)
	}
}

func TestRenderInspectorPaneDoesNotWrapIntoBorder(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	entry, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "elaiia",
		Description:  "test prod baseline study, fix for likert counts",
		StartedAt:    time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 7, 12, 40, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if err := store.UpsertActivitySlots(ctx, []model.ActivitySlot{{
		SlotTime:    time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local).UTC(),
		Operator:    "claude-code",
		MsgCount:    24,
		GitBranch:   "dev",
		Cwd:         "/Users/akoskm/Projects/delta-one-nextjs",
		TokenInput:  36,
		TokenOutput: 12000,
		UserTexts: []string{
			"i have d61c89a36a99d09b899f4da6e102d5567aac3968 and c2fd8bc0ba349f357a432c773f2c2ad81f8f76b8 in main that aren't matching",
			"i marked some discrepancy on the screenshot. this is from study d9588c93-8c9f-4f63-b52c-5b4d0ee43ab2 in dev and it still looks off",
		},
	}}); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.width = 200
	m.height = 30
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.cursor = 0
	m.dayFocusKind = "entry"
	m.focusEntryByID(entry.ID)
	m.loadActivitySlots()
	m.syncInspectorViewport()

	paneWidth := dayInspectorWidth(m.width)
	pane := renderInspectorPane(m, newStyles(m.width), paneWidth, dayPaneHeight(m.height))
	lines := strings.Split(pane, "\n")
	for _, line := range lines[1 : len(lines)-1] {
		if strings.Count(line, "│") > 2 {
			t.Fatalf("inspector line wrapped into border: %q", line)
		}
	}
}

func TestRenderInspectorPaneMatchesRequestedWidth(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.width = 180
	m.height = 20
	m.SetDefaultTimelineView("day")
	m.syncInspectorViewport()

	requested := 90
	pane := renderInspectorPane(m, newStyles(m.width), requested, dayPaneHeight(m.height))
	if lipgloss.Width(pane) != requested {
		t.Fatalf("inspector pane width = %d, want %d\n%s", lipgloss.Width(pane), requested, pane)
	}
}

func TestRenderInspectorPaneShowsScrollbarWhenOverflowing(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	day := time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local)
	slots := make([]model.ActivitySlot, 0, 6)
	for i := 0; i < 6; i++ {
		slots = append(slots, model.ActivitySlot{
			SlotTime:  time.Date(2026, 4, 7, 8, i*15, 0, 0, time.Local).UTC(),
			Operator:  "claude-code",
			MsgCount:  10 + i,
			UserTexts: []string{"prompt " + strconv.Itoa(i+1)},
		})
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(day)
	m.dayFocusKind = "slot"
	m.daySlotStart = time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	m.slotMarkStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.slotMarkSpan = 15 * time.Minute
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 18})
	app := updated.(AppModel)

	viewport := buildInspectorViewport(app, max(20, dayInspectorWidth(app.width)-5), inspectorBodyHeight(dayPaneHeight(app.height)))
	if !inspectorNeedsScrollbar(viewport) {
		t.Fatal("overflowing inspector should require scrollbar")
	}
}

func TestRenderInspectorPaneScrollbarMovesAfterScroll(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	day := time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local)
	slots := make([]model.ActivitySlot, 0, 6)
	for i := 0; i < 6; i++ {
		slots = append(slots, model.ActivitySlot{
			SlotTime:  time.Date(2026, 4, 7, 8, i*15, 0, 0, time.Local).UTC(),
			Operator:  "claude-code",
			MsgCount:  10 + i,
			UserTexts: []string{"prompt " + strconv.Itoa(i+1)},
		})
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(day)
	m.dayFocusKind = "slot"
	m.daySlotStart = time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	m.slotMarkStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.slotMarkSpan = 15 * time.Minute
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 18})
	app := updated.(AppModel)

	beforeOffset := app.inspectorViewport.YOffset
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	app = updated.(AppModel)
	afterOffset := app.inspectorViewport.YOffset
	if afterOffset <= beforeOffset {
		t.Fatalf("inspector viewport offset = %d after pgdown, want > %d", afterOffset, beforeOffset)
	}
	thumbStart, thumbEnd := inspectorScrollbar(app.inspectorViewport.TotalLineCount(), app.inspectorViewport.Height, app.inspectorViewport.YOffset)
	if thumbEnd <= thumbStart {
		t.Fatalf("invalid inspector thumb range: %d..%d", thumbStart, thumbEnd)
	}
}

func TestRenderInspectorPaneScrollbarDoesNotRenderTrackGlyphs(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
	})

	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	day := time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local)
	slots := make([]model.ActivitySlot, 0, 6)
	for i := 0; i < 6; i++ {
		slots = append(slots, model.ActivitySlot{
			SlotTime:  time.Date(2026, 4, 7, 8, i*15, 0, 0, time.Local).UTC(),
			Operator:  "claude-code",
			MsgCount:  10 + i,
			UserTexts: []string{"prompt " + strconv.Itoa(i+1)},
		})
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(day)
	m.dayFocusKind = "slot"
	m.daySlotStart = time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	m.slotMarkStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.slotMarkSpan = 15 * time.Minute
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 18})
	app := updated.(AppModel)

	pane := renderInspectorPane(app, app.styles, dayInspectorWidth(app.width), dayPaneHeight(app.height))
	plain := stripANSI(pane)
	if strings.Contains(plain, "▐") || strings.Contains(plain, "││") {
		t.Fatalf("inspector pane contains scrollbar glyph clutter:\n%s", plain)
	}
}

func TestRenderInspectorPaneScrollbarFallsBackToVisibleGlyphsWithoutColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
	})

	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	day := time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local)
	slots := make([]model.ActivitySlot, 0, 6)
	for i := 0; i < 6; i++ {
		slots = append(slots, model.ActivitySlot{
			SlotTime:  time.Date(2026, 4, 7, 8, i*15, 0, 0, time.Local).UTC(),
			Operator:  "claude-code",
			MsgCount:  10 + i,
			UserTexts: []string{"prompt " + strconv.Itoa(i+1)},
		})
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(day)
	m.dayFocusKind = "slot"
	m.daySlotStart = time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	m.slotMarkStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.slotMarkSpan = 15 * time.Minute
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 18})
	app := updated.(AppModel)

	pane := renderInspectorPane(app, app.styles, dayInspectorWidth(app.width), dayPaneHeight(app.height))
	plain := stripANSI(pane)
	if !strings.Contains(plain, "│") {
		t.Fatalf("inspector pane missing visible scrollbar fallback:\n%s", plain)
	}
}

func TestRenderScrollbarColumnFallsBackToGlyphsWithoutColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
	})

	trackStyle := lipgloss.NewStyle().Background(lipgloss.Color("236"))
	thumbStyle := lipgloss.NewStyle().Background(lipgloss.Color("4"))
	got := stripANSI(renderScrollbarColumn(4, 1, 3, trackStyle, thumbStyle))
	if got != "│\n█\n█\n│" {
		t.Fatalf("renderScrollbarColumn() = %q, want visible glyph fallback", got)
	}
}

func TestRenderDayScrollbarDoesNotRenderTrackGlyphs(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
	})

	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "p",
		Description:  "focused block",
		StartedAt:    time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 14, 12, 40, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 14, 0, 0, 0, 0, time.Local))
	m.width = 120
	m.height = 24
	m.focusCurrentEntryInDayView()

	bar := renderDayScrollbar(m, newStyles(m.width))
	plain := stripANSI(bar)
	if strings.Contains(plain, "│") || strings.Contains(plain, "▐") {
		t.Fatalf("day scrollbar contains glyphs:\n%s", plain)
	}
	dayEntries := dayEntriesForDate(m.entries, m.displayedDay().Format("2006-01-02"))
	window := dayTimelineWindow(dayEntries, m.displayedDay(), m.dayWindowStart)
	rows := dayTimelineRows(window, dayPaneHeight(m.height))
	thumbStart, thumbEnd := dayScrollbar(len(rows), window.start, m.displayedDay())
	if thumbEnd <= thumbStart {
		t.Fatalf("invalid day thumb range: %d..%d", thumbStart, thumbEnd)
	}
}

func TestRenderDayScrollbarFallsBackToVisibleGlyphsWithoutColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
	})

	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "p",
		Description:  "focused block",
		StartedAt:    time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
		EndedAt:      time.Date(2026, 4, 14, 12, 40, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 14, 0, 0, 0, 0, time.Local))
	m.width = 120
	m.height = 24
	m.focusCurrentEntryInDayView()

	bar := renderDayScrollbar(m, newStyles(m.width))
	plain := stripANSI(bar)
	if !strings.Contains(plain, "│") {
		t.Fatalf("day scrollbar missing visible fallback glyphs:\n%s", plain)
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
	if strings.Contains(view, "hrs") {
		t.Errorf("unexpected day view header in view:\n%s", view)
	}
	if strings.Contains(view, "Timeline") {
		t.Errorf("unexpected day view title in view:\n%s", view)
	}
}

func TestDayViewHeaderShowsTotalAndProjectBreakdown(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "Code review", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 30, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "hrs", Description: "TUI polish", StartedAt: time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 11, 45, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(app.View())

	checks := []string{
		"total 2h15m",
		"Elaiia 1h30m",
		"hrs 45m",
	}
	for _, want := range checks {
		if !strings.Contains(view, want) {
			t.Fatalf("day header missing %q, got:\n%s", want, view)
		}
	}
}

func TestDayViewShowsLabelForEntryClippedAtTop(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "improve TUI experience",
		StartedAt:    time.Date(2026, 4, 7, 7, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 8, 30, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if err := store.UpsertActivitySlots(ctx, []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 8, 30, 0, 0, time.Local).UTC(), Operator: "claude-code", MsgCount: 10, FirstText: "please create a README.md file with setup instructions"},
	}); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.dayWindowStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(renderDayTimeline(app, newStyles(app.width)))

	entryIdx := strings.Index(view, "improve TUI experience")
	activityIdx := strings.Index(view, "please create a README.md file with setup instructions")
	if entryIdx == -1 {
		t.Fatalf("clipped top entry label missing from day timeline, got:\n%s", view)
	}
	if activityIdx == -1 {
		t.Fatalf("activity marker missing from day timeline, got:\n%s", view)
	}
	if entryIdx > activityIdx {
		t.Fatalf("clipped top entry label rendered after later activity marker, got:\n%s", view)
	}
}

func TestDayViewShowsClippedTopEntryLabelOnlyOnce(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "improve TUI experience",
		StartedAt:    time.Date(2026, 4, 7, 7, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 8, 30, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.dayWindowStart = time.Date(2026, 4, 7, 7, 0, 0, 0, time.Local).Add(time.Hour)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(renderDayTimeline(app, newStyles(app.width)))
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	rowsView := strings.Join(lines[4:len(lines)-1], "\n")

	if got := strings.Count(rowsView, "improve TUI experience"); got != 1 {
		t.Fatalf("clipped top entry label count = %d, want 1, got:\n%s", got, view)
	}
}

func TestDayViewShowsViewportTopEntryLabelOnlyOnce(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "improve TUI experience",
		StartedAt:    time.Date(2026, 4, 7, 7, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 8, 30, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.dayWindowStart = time.Date(2026, 4, 7, 7, 0, 0, 0, time.Local)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(renderDayTimeline(app, newStyles(app.width)))
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	rowsView := strings.Join(lines[4:len(lines)-1], "\n")

	if got := strings.Count(rowsView, "improve TUI experience"); got != 1 {
		t.Fatalf("viewport-top entry label count = %d, want 1, got:\n%s", got, view)
	}
}

func TestDayViewThreeRowEntryRendersTitleInsideBody(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "restore top border title",
		StartedAt:    time.Date(2026, 4, 7, 18, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 18, 45, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.dayWindowStart = time.Date(2026, 4, 7, 18, 0, 0, 0, time.Local)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(renderDayTimeline(app, newStyles(app.width)))

	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	var firstRow string
	var secondRow string
	for i, line := range lines {
		if strings.HasPrefix(line, "18:00") {
			firstRow = line
			if i+1 < len(lines) {
				secondRow = lines[i+1]
			}
			break
		}
	}
	if firstRow == "" {
		t.Fatalf("missing 18:00 row, got:\n%s", view)
	}
	if strings.Contains(firstRow, "restore top border title") {
		t.Fatalf("18:00 row = %q, want no title on top border for three-row entry, got:\n%s", firstRow, view)
	}
	if !strings.Contains(secondRow, "restore top border title") {
		t.Fatalf("row after 18:00 = %q, want title inside body, got:\n%s", secondRow, view)
	}
	rowsView := strings.Join(lines[4:len(lines)-1], "\n")
	if got := strings.Count(rowsView, "restore top border title"); got != 1 {
		t.Fatalf("title count = %d, want 1, got:\n%s", got, view)
	}
}

func TestDayViewSingleSlotEntryKeepsLeadingBorderBeforeTitle(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "prepare for Marc interview",
		StartedAt:    time.Date(2026, 4, 7, 13, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 13, 15, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.dayWindowStart = time.Date(2026, 4, 7, 13, 0, 0, 0, time.Local)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(renderDayTimeline(app, newStyles(app.width)))

	var firstRow string
	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if strings.HasPrefix(line, "13:00") {
			firstRow = line
			break
		}
	}
	if firstRow == "" {
		t.Fatalf("missing 13:00 row, got:\n%s", view)
	}
	if !strings.Contains(firstRow, "┌─prepare for Marc interview") {
		t.Fatalf("13:00 row = %q, want leading border segment before single-slot title, got:\n%s", firstRow, view)
	}
}

func TestDayViewViewportTopEntryKeepsLeadingBorderBeforeTitle(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "prepare for Marc interview",
		StartedAt:    time.Date(2026, 4, 7, 13, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 13, 30, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.dayWindowStart = time.Date(2026, 4, 7, 13, 0, 0, 0, time.Local)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(renderDayTimeline(app, newStyles(app.width)))

	var firstRow string
	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if strings.HasPrefix(line, "13:00") {
			firstRow = line
			break
		}
	}
	if firstRow == "" {
		t.Fatalf("missing 13:00 row, got:\n%s", view)
	}
	if !strings.Contains(firstRow, "┌─prepare for Marc interview") {
		t.Fatalf("13:00 row = %q, want leading border segment before viewport-top title, got:\n%s", firstRow, view)
	}
}

func TestDayViewMultiRowEntryShowsTitleInsideBody(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "michal interview",
		StartedAt:    time.Date(2026, 4, 7, 10, 42, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 11, 58, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.dayWindowStart = time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(renderDayTimeline(app, newStyles(app.width)))

	var topRow string
	var bodyRow string
	for i, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if strings.HasPrefix(line, "10:00") {
			topRow = line
		}
		if strings.HasPrefix(line, "11:00") {
			if i+1 < len(strings.Split(strings.TrimRight(view, "\n"), "\n")) {
				bodyRow = strings.Split(strings.TrimRight(view, "\n"), "\n")[i+1]
			}
		}
	}
	if topRow == "" || bodyRow == "" {
		t.Fatalf("missing expected rows, got:\n%s", view)
	}
	if strings.Contains(topRow, "michal interview") {
		t.Fatalf("10:00 row = %q, want no title on top border for multi-row entry, got:\n%s", topRow, view)
	}
	if !strings.Contains(bodyRow, "michal interview") {
		t.Fatalf("body row = %q, want title inside body for multi-row entry, got:\n%s", bodyRow, view)
	}
}

func TestDayViewThreeRowEntryShowsTitleInsideBody(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "prepare for Marc interview",
		StartedAt:    time.Date(2026, 4, 7, 13, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 13, 45, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.dayWindowStart = time.Date(2026, 4, 7, 13, 0, 0, 0, time.Local)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(renderDayTimeline(app, newStyles(app.width)))

	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	var topRow string
	var secondRow string
	for i, line := range lines {
		if strings.HasPrefix(line, "13:00") {
			topRow = line
			if i+1 < len(lines) {
				secondRow = lines[i+1]
			}
			break
		}
	}
	if topRow == "" || secondRow == "" {
		t.Fatalf("missing expected rows, got:\n%s", view)
	}
	if strings.Contains(topRow, "prepare for Marc interview") {
		t.Fatalf("13:00 row = %q, want no title on top border for three-row entry, got:\n%s", topRow, view)
	}
	if !strings.Contains(secondRow, "prepare for Marc interview") {
		t.Fatalf("row after 13:00 = %q, want title inside body for three-row entry, got:\n%s", secondRow, view)
	}
}

func TestDayViewFocusedThreeRowEntryTouchingAboveShowsTitleInsideBlock(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "entry above",
		StartedAt:    time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	created, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "freshly created block",
		StartedAt:    time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 10, 45, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.dayWindowStart = time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local)
	m.focusEntryByID(created.ID)
	m.dayFocusKind = "entry"
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(renderDayTimeline(app, newStyles(app.width)))

	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	var firstRow string
	var secondRow string
	for i, line := range lines {
		if strings.HasPrefix(line, "10:00") {
			firstRow = line
			if i+1 < len(lines) {
				secondRow = lines[i+1]
			}
			break
		}
	}
	if firstRow == "" {
		t.Fatalf("missing 10:00 row, got:\n%s", view)
	}
	if strings.Contains(firstRow, "freshly created block") {
		t.Fatalf("10:00 row = %q, want no title in top border row, got:\n%s", firstRow, view)
	}
	if !strings.Contains(firstRow, "10:00 ┌") {
		t.Fatalf("10:00 row = %q, want fresh top border on lower block start row, got:\n%s", firstRow, view)
	}
	if !strings.Contains(secondRow, "freshly created block") {
		t.Fatalf("row after 10:00 = %q, want title inside body, got:\n%s", secondRow, view)
	}
	rowsView := strings.Join(lines[4:len(lines)-1], "\n")
	if got := strings.Count(rowsView, "freshly created block"); got != 1 {
		t.Fatalf("title count = %d, want 1, got:\n%s", got, view)
	}
}

func TestDayViewRendersOverlappingEntriesSideBySide(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "product meeting",
		StartedAt:    time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "claude debugging",
		StartedAt:    time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 9, 45, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.dayWindowStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	app := updated.(AppModel)
	view := stripANSI(renderDayTimeline(app, newStyles(app.width)))

	var topRow string
	var bodyRow string
	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if strings.Contains(line, "09:00") && strings.Contains(line, "claude debugging") {
			topRow = line
		}
		if strings.Contains(line, "product meeting") {
			bodyRow = line
		}
	}
	if !strings.Contains(topRow, "09:00") || !strings.Contains(topRow, "claude debugging") {
		t.Fatalf("overlap top row missing split lanes, got:\n%s", view)
	}
	if !strings.Contains(bodyRow, "product meeting") || !strings.Contains(bodyRow, "│ │") {
		t.Fatalf("overlap row missing side-by-side labels, got:\n%s", view)
	}
}

func TestEnterOnSlotWithMultipleOverlapsOpensChooser(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "product meeting",
		StartedAt:    time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{
		ProjectIdent: "hrs",
		Description:  "claude debugging",
		StartedAt:    time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local),
		EndedAt:      time.Date(2026, 4, 7, 9, 45, 0, 0, time.Local),
	}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.dayWindowStart = time.Date(2026, 4, 7, 8, 0, 0, 0, time.Local)
	m.dayFocusKind = "slot"
	m.daySlotStart = time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local)
	m.daySlotSpan = 15 * time.Minute
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 35})
	app := updated.(AppModel)

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)

	if app.mode != modeOverlapChooser {
		t.Fatalf("mode after enter = %q, want %q", app.mode, modeOverlapChooser)
	}
	view := stripANSI(app.View())
	if !strings.Contains(view, "Choose Overlap") {
		t.Fatalf("chooser title missing, got:\n%s", view)
	}
	if !strings.Contains(view, "product meeting") || !strings.Contains(view, "claude debugging") {
		t.Fatalf("chooser entries missing, got:\n%s", view)
	}
}

func TestDayViewScrollbarUsesStyledGutter(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
	})

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
	if strings.Contains(view, "▐") {
		t.Fatalf("styled gutter scrollbar should not leak glyph clutter:\n%s", view)
	}
}

func TestFocusedAssignedEntryDoesNotBleedBackgroundIntoScrollbarGutters(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "Elaiia", Code: "elaiia", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "test prod baseline study, fix for likert counts", StartedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 14, 12, 50, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	entry, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "elaiia", Description: "run validation protocol on baseline study, analyze results", StartedAt: time.Date(2026, 4, 14, 16, 15, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 14, 18, 15, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	for _, input := range []db.ManualEntryInput{
		{ProjectIdent: "elaiia", Description: "prepare for interview", StartedAt: time.Date(2026, 4, 14, 14, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 14, 14, 45, 0, 0, time.UTC)},
		{ProjectIdent: "elaiia", Description: "magnus interview", StartedAt: time.Date(2026, 4, 14, 15, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 14, 15, 30, 0, 0, time.UTC)},
		{ProjectIdent: "elaiia", Description: "restore & verify user in prod", StartedAt: time.Date(2026, 4, 14, 15, 30, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 14, 15, 50, 0, 0, time.UTC)},
	} {
		if _, err := store.CreateManualEntry(ctx, input); err != nil {
			t.Fatalf("CreateManualEntry() error = %v", err)
		}
	}

	slots := make([]model.ActivitySlot, 0, 7)
	for i := 0; i < 7; i++ {
		slots = append(slots, model.ActivitySlot{
			SlotTime:    time.Date(2026, 4, 14, 16, i*15, 0, 0, time.Local).UTC(),
			Operator:    "claude-code",
			MsgCount:    40 + i,
			GitBranch:   "dev",
			Cwd:         "/Users/akoskm/Projects/delta-one-nextjs",
			TokenInput:  50 + i,
			TokenOutput: 16000 + i,
			UserTexts: []string{
				"looking at ~/Downloads/statistic_id439557_leading-womens-underwear-brands-in-france-2023-by-number-of-users.xlsx",
				"saved Validation Criteria Protocol 10042026 into tmp, check that one",
				"okay, now if I'd tell you we have a study in dev d9588c93-8c9f-4f63-b52c-5b4d0ee43ab2, could you figure out what's wrong",
			},
		})
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 14, 0, 0, 0, 0, time.Local))
	m.width = 160
	m.height = 30
	m.focusEntryByID(entry.ID)
	m.dayFocusKind = "entry"
	m.loadActivitySlots()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	app := updated.(AppModel)

	dayEntries := dayEntriesForDate(app.entries, app.displayedDay().Format("2006-01-02"))
	window := dayTimelineWindow(dayEntries, app.displayedDay(), app.dayWindowStart)
	rows := dayTimelineRows(window, dayPaneHeight(app.height))
	selected := app.entries[app.cursor]
	entryEnd := timelineBlockEnd(selected)
	firstRow := -1
	lastRow := -1
	for i, row := range rows {
		if rangesOverlap(row.start, row.end, selected.StartedAt, entryEnd) {
			if firstRow == -1 {
				firstRow = i
			}
			lastRow = i
		}
	}
	if firstRow == -1 {
		t.Fatal("selected entry did not map to any visible day rows")
	}

	view := app.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	selectedBG := "\x1b[48;5;153m"
	for i := firstRow + 4; i <= lastRow+4 && i < len(lines); i++ {
		if strings.Count(lines[i], selectedBG) > 1 {
			t.Fatalf("selected background bleeds outside entry cell on line %d: %q", i, lines[i])
		}
	}
}

func TestDayScrollbarPosition(t *testing.T) {
	day := time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local)
	// window at start of day
	thumbStart, thumbEnd := dayScrollbar(24, dayStart(day), day)
	if thumbStart != 0 {
		t.Fatalf("thumbStart = %d, want 0 for window at day start", thumbStart)
	}
	if thumbEnd <= thumbStart {
		t.Fatalf("thumbEnd = %d should be > thumbStart = %d", thumbEnd, thumbStart)
	}

	// window at end of day (14:00 start, 10h window)
	thumbStart2, _ := dayScrollbar(24, dayStart(day).Add(14*time.Hour), day)
	if thumbStart2 <= thumbStart {
		t.Fatalf("later window thumbStart = %d should be > early window thumbStart = %d", thumbStart2, thumbStart)
	}
}

func TestJumpToNowScrollsViewportToShowNow(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.InitializeTodayTimelineView()
	m.height = 30

	// scroll window to early morning (03:00-13:00) so "now" might be near the edge or outside
	now := time.Now().In(time.Local)
	m.dayWindowStart = dayStart(now).Add(3 * time.Hour)
	m.daySlotStart = dayStart(now).Add(3 * time.Hour)
	m.dayFocusKind = "slot"

	// press n to jump to now
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	app := updated.(AppModel)

	// the window must contain "now" — and "now" should be in the middle third, not at the edge
	windowEnd := app.dayWindowStart.Add(10 * time.Hour)
	if now.Before(app.dayWindowStart) || !now.Before(windowEnd) {
		t.Fatalf("after n: now %s is outside window %s-%s",
			clock(now), clock(app.dayWindowStart), clock(windowEnd))
	}
	// now should be centered when possible; near day-end clamping may prevent
	// symmetric margins while still keeping now visible.
	marginFromStart := now.Sub(app.dayWindowStart)
	marginFromEnd := windowEnd.Sub(now)
	if dayStart(now).Add(20*time.Hour).After(now) && (marginFromStart < 4*time.Hour || marginFromEnd < 4*time.Hour) {
		t.Fatalf("after n: now not centered — %s from start, %s from end (window %s-%s)",
			marginFromStart, marginFromEnd, clock(app.dayWindowStart), clock(windowEnd))
	}
}

func TestJumpToTodayScrollsViewportToShowNow(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.InitializeTodayTimelineView()
	m.height = 30

	// move to a different day first, then scroll to early morning
	yesterday := dayStart(time.Now()).AddDate(0, 0, -1)
	m.dayDate = yesterday
	m.dayWindowStart = yesterday
	m.daySlotStart = yesterday
	m.dayFocusKind = "slot"

	// press t to jump to today
	now := time.Now().In(time.Local)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	app := updated.(AppModel)

	// must be on today
	if !app.displayedDay().Equal(dayStart(time.Now())) {
		t.Fatalf("after t: displayed day = %s, want today", app.displayedDay().Format("2006-01-02"))
	}

	// the window must contain "now" — and "now" should be roughly centered
	windowEnd := app.dayWindowStart.Add(10 * time.Hour)
	if now.Before(app.dayWindowStart) || !now.Before(windowEnd) {
		t.Fatalf("after t: now %s is outside window %s-%s",
			clock(now), clock(app.dayWindowStart), clock(windowEnd))
	}
	// now should be centered when possible; near day-end clamping may prevent
	// symmetric margins while still keeping now visible.
	marginFromStart := now.Sub(app.dayWindowStart)
	marginFromEnd := windowEnd.Sub(now)
	if dayStart(now).Add(20*time.Hour).After(now) && (marginFromStart < 4*time.Hour || marginFromEnd < 4*time.Hour) {
		t.Fatalf("after t: now not centered — %s from start, %s from end (window %s-%s)",
			marginFromStart, marginFromEnd, clock(app.dayWindowStart), clock(windowEnd))
	}
}

func TestMouseScrollShiftsViewportAndSnapsSlot(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.InitializeTodayTimelineView()
	m.height = 30
	// set window to 08:00-18:00
	today := dayStart(time.Now())
	m.dayWindowStart = today.Add(8 * time.Hour)
	m.daySlotStart = today.Add(10 * time.Hour)
	m.daySlotSpan = 15 * time.Minute
	m.dayFocusKind = "slot"
	startWindow := m.dayWindowStart

	// scroll down — window should shift forward by 1h
	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	app := updated.(AppModel)
	if !app.dayWindowStart.After(startWindow) {
		t.Fatalf("scroll down: window did not shift forward, still at %s", clock(app.dayWindowStart))
	}

	// scroll up — window should shift back
	prevWindow := app.dayWindowStart
	updated, _ = app.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	app = updated.(AppModel)
	if !app.dayWindowStart.Before(prevWindow) {
		t.Fatalf("scroll up: window did not shift back, still at %s", clock(app.dayWindowStart))
	}

	// slot should stay within visible window after scroll
	windowEnd := app.dayWindowStart.Add(10 * time.Hour)
	if app.daySlotStart.Before(app.dayWindowStart) || !app.daySlotStart.Before(windowEnd) {
		t.Fatalf("slot %s is outside window %s-%s after scroll",
			clock(app.daySlotStart), clock(app.dayWindowStart), clock(windowEnd))
	}

	// scroll past end of day — window should clamp, slot should snap
	for i := 0; i < 20; i++ {
		updated, _ = app.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
		app = updated.(AppModel)
	}
	windowEnd = app.dayWindowStart.Add(10 * time.Hour)
	if app.daySlotStart.Before(app.dayWindowStart) || !app.daySlotStart.Before(windowEnd) {
		t.Fatalf("slot %s outside window %s-%s after aggressive scroll down",
			clock(app.daySlotStart), clock(app.dayWindowStart), clock(windowEnd))
	}

	// scroll past start of day — same
	for i := 0; i < 30; i++ {
		updated, _ = app.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
		app = updated.(AppModel)
	}
	windowEnd = app.dayWindowStart.Add(10 * time.Hour)
	if app.daySlotStart.Before(app.dayWindowStart) || !app.daySlotStart.Before(windowEnd) {
		t.Fatalf("slot %s outside window %s-%s after aggressive scroll up",
			clock(app.daySlotStart), clock(app.dayWindowStart), clock(windowEnd))
	}
}

func TestSlotMarkerVisibleWhenSteppingFromEntry(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	// create entry at a fixed time
	entryStart := time.Date(2026, 4, 3, 13, 0, 0, 0, time.UTC)
	entryEnd := time.Date(2026, 4, 3, 13, 30, 0, 0, time.UTC)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Some work", StartedAt: entryStart, EndedAt: entryEnd}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	m.height = 30
	m.width = 120
	// focus on the entry
	m.dayFocusKind = "entry"
	m.cursor = 0
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app := updated.(AppModel)

	// press down once — should immediately show slot marker in the view
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = updated.(AppModel)

	if app.dayFocusKind != "slot" {
		t.Fatalf("after down from entry: focus = %q, want slot", app.dayFocusKind)
	}

	view := stripANSI(app.View())
	// the slot marker renders as a highlighted time cell — the activePicker style
	// produces a visible block. Check that the view contains the slot time info
	// in the footer (proves slot is active and positioned)
	if !strings.Contains(view, "slot") {
		t.Fatalf("after down from entry: view missing 'slot' in footer, marker not visible:\n%s", view)
	}

	// critically: the slot must NOT overlap the entry — it must be AFTER it
	entryEndLocal := entryEnd.In(time.Local)
	if app.daySlotStart.Before(entryEndLocal) {
		t.Fatalf("slot at %s overlaps entry ending at %s — marker would be hidden by entry",
			clock(app.daySlotStart), clock(entryEndLocal))
	}

	// the slot marker must also not be hidden by overlappingEntryIndexForSlot
	if app.overlappingEntryIndexForSlot() >= 0 {
		t.Fatalf("overlappingEntryIndexForSlot = %d, want -1 — slot overlaps entry so marker won't render in time cell",
			app.overlappingEntryIndexForSlot())
	}
}

func TestSlotMarkerAlwaysVisibleInTimeCell(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	entryStart := time.Date(2026, 4, 3, 13, 0, 0, 0, time.UTC)
	entryEnd := time.Date(2026, 4, 3, 14, 0, 0, 0, time.UTC)
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Long work", StartedAt: entryStart, EndedAt: entryEnd}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	m.height = 30
	m.width = 120

	// place slot ON TOP of the entry (overlapping)
	m.dayFocusKind = "slot"
	m.daySlotStart = entryStart.In(time.Local)
	m.daySlotSpan = 15 * time.Minute

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app := updated.(AppModel)

	// the slot overlaps the entry
	if app.overlappingEntryIndexForSlot() < 0 {
		t.Skip("slot doesn't overlap entry in this timezone — test not applicable")
	}

	// even when overlapping an entry, the time cell should still show the marker
	dayStr := app.displayedDay().Format("2006-01-02")
	dayEntries := dayEntriesForDate(app.entries, dayStr)
	window := dayTimelineWindow(dayEntries, app.displayedDay(), app.dayWindowStart)
	rows := dayTimelineRows(window, app.height)

	// verify the slot overlaps a row (proving the marker branch is reachable)
	slotStart := app.daySlotStart
	slotEnd := app.daySlotStart.Add(app.daySlotSpan)
	overlappingRows := 0
	for _, row := range rows {
		if rangesOverlap(row.start, row.end, slotStart, slotEnd) {
			overlappingRows++
		}
	}
	if overlappingRows == 0 {
		t.Fatal("no rows overlap the slot — marker can't render")
	}

	// verify timeCellIsSlotHighlighted returns true even when slot overlaps an entry
	highlighted := false
	for _, row := range rows {
		if rangesOverlap(row.start, row.end, slotStart, slotEnd) && timeCellIsSlotHighlighted(app, row) {
			highlighted = true
			break
		}
	}
	if !highlighted {
		t.Fatal("timeCellIsSlotHighlighted returned false for slot overlapping entry — marker won't render")
	}
}

func TestShiftUpDownSnapsToHourThenJumps(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.InitializeTodayTimelineView()
	m.height = 30
	m.width = 120
	// position slot at 10:23 (not on a full hour)
	today := dayStart(time.Now())
	m.dayWindowStart = today.Add(6 * time.Hour)
	m.daySlotStart = today.Add(10*time.Hour + 23*time.Minute)
	m.daySlotSpan = 15 * time.Minute
	m.dayFocusKind = "slot"

	// shift+down from 10:23 should snap to 11:00 first, then +1h = 12:00
	// actually: snap to next full hour (11:00), then add delta (1h) = 12:00
	// wait — the logic snaps to 11:00 (next hour since going down), then adds 1h = 12:00
	// hmm that's 2h jump. Let me re-read: snap to nearest full hour, then jump.
	// going down from 10:23: snap to 11:00 (next hour), add +1h = 12:00? No.
	// The intent is: first press goes to closest full hour, then subsequent presses go 1h.
	// So first shift+down from 10:23 → 11:00
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	app := updated.(AppModel)
	if clock(app.daySlotStart) != "11:00" {
		t.Fatalf("shift+down from 10:23: slot = %s, want 11:00", clock(app.daySlotStart))
	}

	// second shift+down: 11:00 → 12:00
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	app = updated.(AppModel)
	if clock(app.daySlotStart) != "12:00" {
		t.Fatalf("2nd shift+down: slot = %s, want 12:00", clock(app.daySlotStart))
	}

	// shift+up: 12:00 → 11:00
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	app = updated.(AppModel)
	if clock(app.daySlotStart) != "11:00" {
		t.Fatalf("shift+up: slot = %s, want 11:00", clock(app.daySlotStart))
	}
}

func TestJKOnlyNavigatesEntries(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	// two entries with a gap between them
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Entry A", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Entry B", StartedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 13, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	m.height = 30
	m.width = 120

	// focus on first entry
	m.dayFocusKind = "entry"
	m.cursor = 0
	if m.entries[0].Description == nil || *m.entries[0].Description != "Entry A" {
		// entries are sorted desc, so Entry B might be first
		if *m.entries[0].Description == "Entry B" {
			m.cursor = 1
		}
	}
	startDesc := *m.entries[m.cursor].Description

	// j should jump directly to the next entry, skipping the gap
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app := updated.(AppModel)

	if app.dayFocusKind != "entry" {
		t.Fatalf("after j: focus = %q, want entry (should never focus gap/slot)", app.dayFocusKind)
	}
	if app.entries[app.cursor].Description == nil || *app.entries[app.cursor].Description == startDesc {
		t.Fatalf("after j: still on same entry %q, should have moved to the other", startDesc)
	}

	// k should jump back to the first entry
	nextDesc := *app.entries[app.cursor].Description
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	app = updated.(AppModel)

	if app.dayFocusKind != "entry" {
		t.Fatalf("after k: focus = %q, want entry", app.dayFocusKind)
	}
	if *app.entries[app.cursor].Description == nextDesc {
		t.Fatalf("after k: still on %q, should have moved back", nextDesc)
	}
}

func TestJKScrollsViewportToShowFocusedEntry(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	// entry A at 02:00 — will be outside the default window
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Early work", StartedAt: time.Date(2026, 4, 3, 2, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 3, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}
	// entry B at 14:00 — will be outside if window starts at 02:00
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "Late work", StartedAt: time.Date(2026, 4, 3, 14, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 15, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("CreateManualEntry() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local))
	m.height = 30
	m.width = 120

	// focus on "Early work" (02:00 UTC = 04:00 local in CEST)
	// set window to show this entry
	earlyLocal := time.Date(2026, 4, 3, 2, 0, 0, 0, time.UTC).In(time.Local)
	m.dayWindowStart = clampDayWindowStart(alignWindowStart(earlyLocal), m.displayedDay())
	indices := m.dayEntryIndices(m.displayedDay().Format("2006-01-02"))
	// find the early entry
	for _, idx := range indices {
		if m.entries[idx].Description != nil && *m.entries[idx].Description == "Early work" {
			m.cursor = idx
			break
		}
	}
	m.dayFocusKind = "entry"

	lateLocal := time.Date(2026, 4, 3, 14, 0, 0, 0, time.UTC).In(time.Local)

	// press j to jump to "Late work"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app := updated.(AppModel)

	if app.entries[app.cursor].Description == nil || *app.entries[app.cursor].Description != "Late work" {
		t.Fatalf("after j: cursor on %q, want Late work", descriptionOrID(app.entries[app.cursor]))
	}

	// the window must now contain the Late work entry
	windowEnd := app.dayWindowStart.Add(10 * time.Hour)
	if lateLocal.Before(app.dayWindowStart) || !lateLocal.Before(windowEnd) {
		t.Fatalf("after j: Late work at %s outside window %s-%s — viewport didn't scroll",
			clock(lateLocal), clock(app.dayWindowStart), clock(windowEnd))
	}

	// entry should be roughly centered (at least 2h from edges)
	marginFromStart := lateLocal.Sub(app.dayWindowStart)
	marginFromEnd := windowEnd.Sub(lateLocal)
	if marginFromStart < 2*time.Hour || marginFromEnd < 2*time.Hour {
		t.Fatalf("entry not centered: %s from start, %s from end (window %s-%s)",
			marginFromStart, marginFromEnd, clock(app.dayWindowStart), clock(windowEnd))
	}
}

