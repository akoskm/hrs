package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"
)

func setupInboxTest(t *testing.T, slots []model.ActivitySlot, day time.Time, setup func(ctx context.Context, store *db.Store)) (context.Context, *db.Store, AppModel) {
	t.Helper()
	ctx := context.Background()
	store := openTestStore(t)
	t.Cleanup(func() { store.Close() })
	if setup != nil {
		setup(ctx, store)
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}
	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(day)
	m.openInbox()
	return ctx, store, m
}

func TestInboxOpensAndShowsUncoveredSlots(t *testing.T) {
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

	// insert activity slots: one covered by entry, one not
	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local), Operator: "claude-code", Cwd: "/tmp/hrs", MsgCount: 5, FirstText: "covered"},
		{SlotTime: time.Date(2026, 4, 7, 11, 0, 0, 0, time.Local), Operator: "claude-code", Cwd: "/tmp/hrs", MsgCount: 3, FirstText: "uncovered"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.SetDefaultTimelineView("day")
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.height = 30
	m.width = 120

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	app := updated.(AppModel)
	if app.mode != modeInbox {
		t.Fatalf("mode = %q, want inbox", app.mode)
	}
	if len(app.inboxItems) != 1 {
		t.Fatalf("inbox items = %d, want 1", len(app.inboxItems))
	}
	view := stripANSI(app.View())
	if !strings.Contains(view, "uncovered") {
		t.Fatalf("inbox missing uncovered slot: %q", view)
	}
	if strings.Contains(view, "09:15") {
		t.Fatalf("inbox should not show covered 09:15 slot: %q", view)
	}
}

func TestInboxMergesConsecutiveSlots(t *testing.T) {
	_, _, m := setupInboxTest(t, []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "a"},
		{SlotTime: time.Date(2026, 4, 7, 9, 15, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "b"},
		{SlotTime: time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local), Operator: "codex", MsgCount: 1, FirstText: "c"},
	}, time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local), nil)

	if len(m.inboxItems) != 2 {
		t.Fatalf("inbox items = %d, want 2 (merged a+b + c)", len(m.inboxItems))
	}
	if m.inboxItems[0].MsgCount != 2 {
		t.Fatalf("merged item msg count = %d, want 2", m.inboxItems[0].MsgCount)
	}
}

func TestInboxWeekMonthFilter(t *testing.T) {
	_, _, m := setupInboxTest(t, []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1},
		{SlotTime: time.Date(2026, 4, 14, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1},
	}, time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local), nil)

	if len(m.inboxItems) != 1 {
		t.Fatalf("week inbox items = %d, want 1", len(m.inboxItems))
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	app := updated.(AppModel)
	if app.inboxPreset != inboxPresetMonth {
		t.Fatalf("preset = %q, want month", app.inboxPreset)
	}
	if len(app.inboxItems) != 2 {
		t.Fatalf("month inbox items = %d, want 2", len(app.inboxItems))
	}
}

func TestInboxCreatesEntryOnEnter(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", Cwd: "/tmp/hrs", MsgCount: 1, FirstText: "debug auth"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	if len(m.inboxItems) != 1 {
		t.Fatalf("inbox items = %d, want 1", len(m.inboxItems))
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)

	if app.mode != modeEntryEdit {
		t.Fatalf("mode after enter = %q, want entry-edit", app.mode)
	}

	entries, err := store.ListEntries(ctx)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	found := false
	for _, entry := range entries {
		if entry.Description != nil && strings.Contains(*entry.Description, "debug auth") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("entry not created from inbox item: %#v", entries)
	}
	if len(app.inboxItems) != 1 {
		t.Fatalf("inbox items after enter = %d, want 1 (not removed until saved)", len(app.inboxItems))
	}

	// save the entry (tab to project, j to select, enter to save)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)

	if app.mode != modeInbox {
		t.Fatalf("mode after save = %q, want inbox", app.mode)
	}
	if len(app.inboxItems) != 0 {
		t.Fatalf("inbox items after save = %d, want 0", len(app.inboxItems))
	}
}

func TestInboxDismissRemovesItem(t *testing.T) {
	_, _, m := setupInboxTest(t, []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "noise"},
	}, time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local), nil)

	if len(m.inboxItems) != 1 {
		t.Fatalf("inbox items = %d, want 1", len(m.inboxItems))
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	app := updated.(AppModel)

	if len(app.inboxItems) != 0 {
		t.Fatalf("inbox items after dismiss = %d, want 0", len(app.inboxItems))
	}
}

func TestInboxEmptyState(t *testing.T) {
	_, _, m := setupInboxTest(t, nil, time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local), nil)

	if len(m.inboxItems) != 0 {
		t.Fatalf("inbox items = %d, want 0", len(m.inboxItems))
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "No uncategorized activity") {
		t.Fatalf("empty state missing: %q", view)
	}
}

func TestInboxAssignReturnsToInbox(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", Cwd: "/tmp/hrs", MsgCount: 1, FirstText: "debug auth"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	// enter creates entry and opens full edit dialog
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	if app.mode != modeEntryEdit {
		t.Fatalf("mode after enter = %q, want entry-edit", app.mode)
	}

	// select first project and save
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)

	if app.mode != modeInbox {
		t.Fatalf("mode after assign = %q, want inbox", app.mode)
	}
}

func TestInboxSearchFiltersItems(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "debug auth"},
		{SlotTime: time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local), Operator: "codex", MsgCount: 1, FirstText: "fix billing"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	if len(m.inboxItems) != 2 {
		t.Fatalf("inbox items = %d, want 2", len(m.inboxItems))
	}

	// open search
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)
	if app.mode != modeInbox {
		t.Fatalf("mode after / = %q, want inbox", app.mode)
	}
	if !app.inboxSearchActive {
		t.Fatalf("inboxSearchActive = false, want true")
	}

	// type query - should filter live
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("billing")})
	app = updated.(AppModel)
	
	if len(app.inboxItems) != 1 {
		t.Fatalf("inbox items after search = %d, want 1", len(app.inboxItems))
	}
	if !strings.Contains(app.inboxItems[0].Texts[0], "billing") {
		t.Fatalf("inbox item not matching search: %q", app.inboxItems[0].Texts[0])
	}

	// confirm with enter — opens edit dialog
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	
	if app.mode != modeEntryEdit {
		t.Fatalf("mode after enter = %q, want entry-edit", app.mode)
	}
	if app.inboxSearchActive {
		t.Fatalf("inboxSearchActive = true, want false after enter")
	}

	// cancel dialog — filter should persist
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)

	if app.mode != modeInbox {
		t.Fatalf("mode after esc = %q, want inbox", app.mode)
	}
	if len(app.inboxItems) != 1 {
		t.Fatalf("filtered items after cancel = %d, want 1", len(app.inboxItems))
	}
	if app.inboxLastSearch != "billing" {
		t.Fatalf("inboxLastSearch = %q, want billing", app.inboxLastSearch)
	}
}

func TestInboxSearchSlashDoesNotSwitchToTimeline(t *testing.T) {
	_, _, m := setupInboxTest(t, []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "debug auth"},
	}, time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local), nil)

	if m.mode != modeInbox {
		t.Fatalf("mode before / = %q, want inbox", m.mode)
	}

	// press / to open search
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)

	if app.mode != modeInbox {
		t.Fatalf("mode after / = %q, want inbox", app.mode)
	}
	if !app.inboxSearchActive {
		t.Fatalf("inboxSearchActive = false, want true")
	}
}

func TestInboxLiveSearchFiltersAsYouType(t *testing.T) {
	_, _, m := setupInboxTest(t, []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "debug auth"},
		{SlotTime: time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local), Operator: "codex", MsgCount: 1, FirstText: "fix billing"},
	}, time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local), nil)

	if len(m.inboxItems) != 2 {
		t.Fatalf("inbox items = %d, want 2", len(m.inboxItems))
	}

	// press / to activate search
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)
	
	if app.mode != modeInbox {
		t.Fatalf("mode after / = %q, want inbox", app.mode)
	}
	if !app.inboxSearchActive {
		t.Fatalf("inboxSearchActive = false, want true")
	}

	// type 'bill' - should filter to 1 item
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("bill")})
	app = updated.(AppModel)
	
	if len(app.inboxItems) != 1 {
		t.Fatalf("inbox items after typing 'bill' = %d, want 1", len(app.inboxItems))
	}
	if !strings.Contains(app.inboxItems[0].Texts[0], "billing") {
		t.Fatalf("inbox item not matching 'bill': %q", app.inboxItems[0].Texts[0])
	}

	// type 'ing' - should still be 1 item ("billing" matches)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ing")})
	app = updated.(AppModel)
	
	if len(app.inboxItems) != 1 {
		t.Fatalf("inbox items after typing 'billing' = %d, want 1", len(app.inboxItems))
	}

	// esc should clear search and show all items
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)
	
	if app.inboxSearchActive {
		t.Fatalf("inboxSearchActive = true, want false after esc")
	}
	if len(app.inboxItems) != 2 {
		t.Fatalf("inbox items after esc = %d, want 2", len(app.inboxItems))
	}
}

func TestInboxLiveSearchAllowsSpace(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "debug auth"},
		{SlotTime: time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local), Operator: "codex", MsgCount: 1, FirstText: "fix billing"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	// activate search
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)

	// type "fix " with a trailing space
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fix")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeySpace})
	app = updated.(AppModel)

	if app.inboxSearchQuery != "fix " {
		t.Fatalf("search query = %q, want %q", app.inboxSearchQuery, "fix ")
	}
	if len(app.inboxItems) != 1 {
		t.Fatalf("inbox items = %d, want 1", len(app.inboxItems))
	}
}

func TestInboxSearchNavigationInFilteredList(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "debug auth"},
		{SlotTime: time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "fix billing"},
		{SlotTime: time.Date(2026, 4, 7, 11, 0, 0, 0, time.Local), Operator: "codex", MsgCount: 1, FirstText: "deploy auth"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	// activate search and type "auth" to filter to 2 items
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("auth")})
	app = updated.(AppModel)

	if len(app.inboxItems) != 2 {
		t.Fatalf("filtered items = %d, want 2", len(app.inboxItems))
	}
	if app.inboxCursor != 0 {
		t.Fatalf("cursor = %d, want 0", app.inboxCursor)
	}

	// navigate down while search is active (use arrow keys, not j/k)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = updated.(AppModel)

	if app.inboxCursor != 1 {
		t.Fatalf("cursor after down = %d, want 1", app.inboxCursor)
	}
	if !app.inboxSearchActive {
		t.Fatalf("search should still be active")
	}

	// navigate up
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyUp})
	app = updated.(AppModel)

	if app.inboxCursor != 0 {
		t.Fatalf("cursor after up = %d, want 0", app.inboxCursor)
	}
}

func TestInboxSearchAllowsTypingKAndJ(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "debug auth"},
		{SlotTime: time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local), Operator: "codex", MsgCount: 1, FirstText: "fix billing"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	// activate search
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)

	// type "kj" — both letters should appear in query
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = updated.(AppModel)

	if app.inboxSearchQuery != "kj" {
		t.Fatalf("search query = %q, want %q", app.inboxSearchQuery, "kj")
	}
	if app.inboxCursor != 0 {
		t.Fatalf("cursor should not move while typing, got %d", app.inboxCursor)
	}
}

func TestInboxCancelAssignDoesNotRemoveItem(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", Cwd: "/tmp/hrs", MsgCount: 1, FirstText: "debug auth"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	if len(m.inboxItems) != 1 {
		t.Fatalf("inbox items = %d, want 1", len(m.inboxItems))
	}

	// enter creates entry and opens full edit dialog
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	if app.mode != modeEntryEdit {
		t.Fatalf("mode after enter = %q, want entry-edit", app.mode)
	}

	entriesBefore, _ := store.ListEntries(ctx)
	if len(entriesBefore) != 1 {
		t.Fatalf("entries before cancel = %d, want 1", len(entriesBefore))
	}

	// press esc to cancel — item should stay in inbox
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)

	if app.mode != modeInbox {
		t.Fatalf("mode after esc = %q, want inbox", app.mode)
	}
	if len(app.inboxItems) != 1 {
		t.Fatalf("inbox items after cancel = %d, want 1", len(app.inboxItems))
	}

	// draft entry should be deleted
	entriesAfter, _ := store.ListEntries(ctx)
	if len(entriesAfter) != 0 {
		t.Fatalf("entries after cancel = %d, want 0", len(entriesAfter))
	}
}

func TestEntryEditCtrlUClearsDescription(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "P", Code: "p", HourlyRate: 100, Currency: "USD"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := store.CreateManualEntry(ctx, db.ManualEntryInput{ProjectIdent: "p", Description: "investigate production bug", StartedAt: time.Date(2026, 4, 3, 9, 0, 0, 0, time.UTC), EndedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("err = %v", err)
	}

	app, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeEntryEdit {
		t.Fatalf("mode = %q, want entry-edit", app.mode)
	}

	// ctrl+u should clear the description
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	app = updated.(AppModel)

	if app.entryInput != "" {
		t.Fatalf("entryInput after ctrl+u = %q, want empty", app.entryInput)
	}
	if app.entryInputCursor != 0 {
		t.Fatalf("entryInputCursor after ctrl+u = %d, want 0", app.entryInputCursor)
	}
}

func TestInboxEnterOpensFullEditDialog(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", Cwd: "/tmp/hrs", MsgCount: 1, FirstText: "debug auth"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)

	if app.mode != modeEntryEdit {
		t.Fatalf("mode after enter = %q, want entry-edit", app.mode)
	}
	if app.entryInput != "debug auth" {
		t.Fatalf("description = %q, want %q", app.entryInput, "debug auth")
	}
	if app.previousMode != modeInbox {
		t.Fatalf("previousMode = %q, want inbox", app.previousMode)
	}

	// esc should return to inbox, item should stay
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)

	if app.mode != modeInbox {
		t.Fatalf("mode after esc = %q, want inbox", app.mode)
	}
	if len(app.inboxItems) != 1 {
		t.Fatalf("inbox items after cancel = %d, want 1", len(app.inboxItems))
	}
}

func TestInboxEnterOnFilteredListOpensEditDialog(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "debug auth"},
		{SlotTime: time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local), Operator: "codex", MsgCount: 1, FirstText: "fix billing"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	// activate search and filter to 1 item
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("billing")})
	app = updated.(AppModel)

	if len(app.inboxItems) != 1 {
		t.Fatalf("filtered items = %d, want 1", len(app.inboxItems))
	}

	// enter should open edit dialog directly (not just deactivate search)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)

	if app.mode != modeEntryEdit {
		t.Fatalf("mode after enter = %q, want entry-edit", app.mode)
	}
	if app.inboxSearchActive {
		t.Fatalf("search should not be active")
	}
}

func TestInboxFilterPersistsAfterClosingEditDialog(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "debug auth"},
		{SlotTime: time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local), Operator: "codex", MsgCount: 1, FirstText: "fix billing"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	// filter to "billing"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("billing")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)

	// enter confirmed search and opened edit dialog
	if app.mode != modeEntryEdit {
		t.Fatalf("mode after enter = %q, want entry-edit", app.mode)
	}
	if len(app.inboxItems) != 1 {
		t.Fatalf("filtered items = %d, want 1", len(app.inboxItems))
	}

	// cancel dialog
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)

	if app.mode != modeInbox {
		t.Fatalf("mode after cancel = %q, want inbox", app.mode)
	}
	if len(app.inboxItems) != 1 {
		t.Fatalf("filtered items after cancel = %d, want 1", len(app.inboxItems))
	}
	if app.inboxLastSearch != "billing" {
		t.Fatalf("inboxLastSearch = %q, want billing", app.inboxLastSearch)
	}
}

func TestInboxFilterCursorPreservedAfterClosingEditDialog(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "debug auth"},
		{SlotTime: time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local), Operator: "codex", MsgCount: 1, FirstText: "fix billing"},
		{SlotTime: time.Date(2026, 4, 7, 11, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "deploy auth"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	// filter to "auth" — should match items 0 and 2
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("auth")})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)

	if len(app.inboxItems) != 2 {
		t.Fatalf("filtered items = %d, want 2", len(app.inboxItems))
	}
	if app.inboxCursor != 0 {
		t.Fatalf("cursor after enter = %d, want 0", app.inboxCursor)
	}

	// move down to second matching item
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)
	if app.mode != modeInbox {
		t.Fatalf("mode after cancel = %q, want inbox", app.mode)
	}
	// navigate to second item before opening dialog again
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = updated.(AppModel)
	if app.inboxCursor != 1 {
		t.Fatalf("cursor after j = %d, want 1", app.inboxCursor)
	}

	// open dialog on second item
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeEntryEdit {
		t.Fatalf("mode after enter = %q, want entry-edit", app.mode)
	}

	// cancel dialog — cursor should return to second item
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)

	if app.mode != modeInbox {
		t.Fatalf("mode after cancel = %q, want inbox", app.mode)
	}
	if len(app.inboxItems) != 2 {
		t.Fatalf("filtered items after cancel = %d, want 2", len(app.inboxItems))
	}
	if app.inboxCursor != 1 {
		t.Fatalf("cursor after cancel = %d, want 1", app.inboxCursor)
	}
	if app.inboxItems[app.inboxCursor].Texts[0] != "deploy auth" {
		t.Fatalf("item at cursor = %q, want deploy auth", app.inboxItems[app.inboxCursor].Texts[0])
	}
}

func TestInboxCursorWithArrowKeysWhileSearchActive(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "debug auth"},
		{SlotTime: time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local), Operator: "codex", MsgCount: 1, FirstText: "fix billing"},
		{SlotTime: time.Date(2026, 4, 7, 11, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "deploy auth"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	// open search, type "auth"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("auth")})
	app = updated.(AppModel)

	if len(app.inboxItems) != 2 {
		t.Fatalf("filtered items = %d, want 2", len(app.inboxItems))
	}
	if app.inboxCursor != 0 {
		t.Fatalf("cursor after search = %d, want 0", app.inboxCursor)
	}

	// navigate down with arrow key while search is still active
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = updated.(AppModel)
	if app.inboxCursor != 1 {
		t.Fatalf("cursor after down = %d, want 1", app.inboxCursor)
	}
	if !app.inboxSearchActive {
		t.Fatalf("search should still be active")
	}

	// press enter to open dialog on second item
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeEntryEdit {
		t.Fatalf("mode after enter = %q, want entry-edit", app.mode)
	}

	// press esc to cancel
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)

	if app.mode != modeInbox {
		t.Fatalf("mode after cancel = %q, want inbox", app.mode)
	}
	if len(app.inboxItems) != 2 {
		t.Fatalf("filtered items after cancel = %d, want 2", len(app.inboxItems))
	}
	if app.inboxCursor != 1 {
		t.Fatalf("cursor after cancel = %d, want 1, got item %q", app.inboxCursor, app.inboxItems[app.inboxCursor].Texts[0])
	}
	if app.inboxItems[app.inboxCursor].Texts[0] != "deploy auth" {
		t.Fatalf("item at cursor = %q, want deploy auth", app.inboxItems[app.inboxCursor].Texts[0])
	}
}

func TestInboxCursorPreservedAfterDismissWithOkaySearch(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	if _, err := store.CreateProject(ctx, db.ProjectCreateInput{Name: "hrs", Code: "hrs", Currency: "CHF"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	slots := []model.ActivitySlot{
		{SlotTime: time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "debug okay"},
		{SlotTime: time.Date(2026, 4, 7, 10, 0, 0, 0, time.Local), Operator: "codex", MsgCount: 1, FirstText: "fix billing"},
		{SlotTime: time.Date(2026, 4, 7, 11, 0, 0, 0, time.Local), Operator: "claude-code", MsgCount: 1, FirstText: "deploy okay"},
	}
	if err := store.UpsertActivitySlots(ctx, slots); err != nil {
		t.Fatalf("UpsertActivitySlots() error = %v", err)
	}

	m, err := NewAppModel(ctx, store)
	if err != nil {
		t.Fatalf("NewAppModel() error = %v", err)
	}
	m.dayDate = dayStart(time.Date(2026, 4, 7, 0, 0, 0, 0, time.Local))
	m.openInbox()

	// filter to "okay" while search active
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("okay")})
	app = updated.(AppModel)

	if len(app.inboxItems) != 2 {
		t.Fatalf("filtered items = %d, want 2", len(app.inboxItems))
	}
	if app.inboxCursor != 0 {
		t.Fatalf("cursor after search = %d, want 0", app.inboxCursor)
	}

	// navigate down to second item while search still active
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = updated.(AppModel)
	if app.inboxCursor != 1 {
		t.Fatalf("cursor after down = %d, want 1", app.inboxCursor)
	}
	if !app.inboxSearchActive {
		t.Fatalf("search should still be active")
	}

	// press enter to open dialog on second item
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.mode != modeEntryEdit {
		t.Fatalf("mode after enter = %q, want entry-edit", app.mode)
	}

	// press esc to dismiss/cancel
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)

	if app.mode != modeInbox {
		t.Fatalf("mode after cancel = %q, want inbox", app.mode)
	}
	if len(app.inboxItems) != 2 {
		t.Fatalf("filtered items after cancel = %d, want 2", len(app.inboxItems))
	}
	if app.inboxCursor != 1 {
		t.Fatalf("cursor after cancel = %d, want 1, got item %q", app.inboxCursor, app.inboxItems[app.inboxCursor].Texts[0])
	}
	if app.inboxItems[app.inboxCursor].Texts[0] != "deploy okay" {
		t.Fatalf("item at cursor = %q, want deploy okay", app.inboxItems[app.inboxCursor].Texts[0])
	}
}
