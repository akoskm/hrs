package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"
	intsync "github.com/akoskm/hrs/internal/sync"
)

type mode string

type projectDialogMode string

type timelineViewMode string

type inspectorTab string

const (
	modeTimeline      mode = "timeline"
	modeAssign        mode = "assign"
	modeEntryEdit     mode = "entry-edit"
	modeGapEntry      mode = "gap-entry"
	modeSearch        mode = "search"
	modeDeleteConfirm mode = "delete-confirm"

	projectDialogAssign projectDialogMode = "assign"
	projectDialogManage projectDialogMode = "manage"
	projectDialogCreate projectDialogMode = "create"

	timelineViewList timelineViewMode = "list"
	timelineViewDay  timelineViewMode = "day"

	inspectorOverview inspectorTab = "overview"
	inspectorSession  inspectorTab = "session"
	inspectorActions  inspectorTab = "actions"
)

type AppModel struct {
	ctx                context.Context
	store              *db.Store
	syncFn             func() error
	allEntries         []model.TimeEntryDetail
	entries            []model.TimeEntryDetail
	projects           []model.Project
	selected           map[string]bool
	searchQuery        string
	lastSearch         string
	sourceFilter       string
	width              int
	height             int
	offset             int
	cursor             int
	projectCursor      int
	gapProjectCursor   int
	entryProjectCursor int
	dialogMode         projectDialogMode
	projectInput       string
	gapInput           string
	gapStartInput      string
	gapEndInput        string
	gapInputField      string
	entryInput         string
	entryStartInput    string
	entryEndInput      string
	entryInputField    string
	entryProjectOnly   bool
	dayFocusKind       string
	dayGapFocus        int
	daySlotStart       time.Time
	daySlotSpan        time.Duration
	dayDate            time.Time
	dayWindowStart     time.Time
	syncing            bool
	syncFrame          int
	caretVisible       bool
	lastSyncedAt       *time.Time
	syncStatusErr      error
	timelineView       timelineViewMode
	inspectorTab       inspectorTab
	confirmDeleteID    string
	mode               mode
	err                error
	quitting           bool
}

type syncPulseMsg struct{}
type cursorBlinkMsg struct{}

type syncDoneMsg struct {
	err error
}

type timelineRow struct {
	Header     string
	Entry      *model.TimeEntryDetail
	EntryIndex int
}

func NewAppModel(ctx context.Context, store *db.Store) (AppModel, error) {
	return NewAppModelWithSync(ctx, store, nil)
}

func NewAppModelWithSync(ctx context.Context, store *db.Store, syncFn func() error) (AppModel, error) {
	entries, err := store.ListEntries(ctx)
	if err != nil {
		return AppModel{}, err
	}
	projects, err := store.ListProjects(ctx)
	if err != nil {
		return AppModel{}, err
	}
	sorted := sortEntries(entries)
	model := AppModel{ctx: ctx, store: store, syncFn: syncFn, allEntries: sorted, projects: projects, selected: map[string]bool{}, mode: modeTimeline, dialogMode: projectDialogAssign, timelineView: timelineViewList, inspectorTab: inspectorOverview, sourceFilter: "all"}
	model.applySourceFilter()
	return model, nil
}

func (m *AppModel) SetDefaultTimelineView(view timelineViewMode) {
	m.timelineView = view
	if view == timelineViewDay {
		m.focusCurrentEntryInDayView()
		if m.daySlotSpan == 0 {
			m.daySlotSpan = 15 * time.Minute
		}
	}
}

func (m *AppModel) InitializeTodayTimelineView() {
	m.timelineView = timelineViewDay
	today := dayStart(time.Now())
	m.dayDate = today
	m.dayWindowStart = defaultDayWindowStart(today)
	m.daySlotSpan = 15 * time.Minute
	m.daySlotStart = roundDownToStep(time.Now().In(time.Local), 15*time.Minute)
	m.dayFocusKind = "slot"
}

func (m *AppModel) cycleInspectorTab(step int) {
	tabs := []inspectorTab{inspectorOverview, inspectorSession, inspectorActions}
	idx := 0
	for i, tab := range tabs {
		if tab == m.inspectorTab {
			idx = i
			break
		}
	}
	idx = (idx + step + len(tabs)) % len(tabs)
	m.inspectorTab = tabs[idx]
}

func (m AppModel) Init() tea.Cmd { return nil }

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.mode == modeSearch {
			switch msg.String() {
			case "esc":
				m.mode = modeTimeline
				m.searchQuery = ""
			case "enter":
				m.lastSearch = strings.TrimSpace(m.searchQuery)
				m.mode = modeTimeline
				m.searchQuery = ""
				if m.lastSearch != "" {
					m.jumpToSearchMatch(m.cursor, 1, true)
				}
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
				}
			default:
				if msg.Type == tea.KeyRunes {
					m.searchQuery += string(msg.Runes)
				}
			}
			return m, nil
		}
		if m.mode == modeEntryEdit {
			return m, m.handleEntryEditKey(msg)
		}
		if m.mode == modeGapEntry {
			return m, m.handleGapEntryKey(msg)
		}
		if m.mode == modeAssign {
			return m, m.handleProjectDialogKey(msg)
		}
		if m.mode == modeDeleteConfirm {
			return m, m.handleDeleteConfirmKey(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "/":
			m.mode = modeSearch
			m.searchQuery = ""
		case "t":
			if m.timelineView == timelineViewDay {
				m.jumpToToday()
			}
		case "tab":
			if m.mode == modeTimeline {
				m.cycleInspectorTab(1)
			}
		case "shift+tab":
			if m.mode == modeTimeline {
				m.cycleInspectorTab(-1)
			}
		case "f":
			if m.mode == modeTimeline {
				m.cycleSourceFilter(1)
			}
		case "F":
			if m.mode == modeTimeline {
				m.cycleSourceFilter(-1)
			}
		case "up":
			if m.timelineView == timelineViewDay {
				m.moveSlot(-15*time.Minute, 15*time.Minute)
			} else if m.cursor > 0 {
				m.cursor--
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "k":
			if m.timelineView == timelineViewDay {
				m.jumpDayItem(-1)
			} else if m.cursor > 0 {
				m.cursor--
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "left", "h":
			if m.timelineView == timelineViewDay {
				m.shiftDisplayedDay(-1)
			}
		case "right", "l":
			if m.timelineView == timelineViewDay {
				m.shiftDisplayedDay(1)
			}
		case "home":
			if m.timelineView == timelineViewDay {
				day := m.displayedDay()
				m.daySlotSpan = 15 * time.Minute
				m.daySlotStart = dayStart(day)
				m.dayFocusKind = "slot"
				m.ensureSlotVisible()
			} else {
				m.cursor = 0
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "end":
			if m.timelineView == timelineViewDay {
				day := m.displayedDay()
				m.daySlotSpan = 15 * time.Minute
				m.daySlotStart = dayStart(day).Add((24 * time.Hour) - (15 * time.Minute))
				m.dayFocusKind = "slot"
				m.ensureSlotVisible()
			} else if len(m.entries) > 0 {
				m.cursor = len(m.entries) - 1
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "shift+up":
			if m.timelineView == timelineViewDay {
				m.moveSlot(-time.Hour, time.Hour)
			}
		case "shift+down":
			if m.timelineView == timelineViewDay {
				m.moveSlot(time.Hour, time.Hour)
			}
		case "pgdown", "ctrl+f":
			if len(m.entries) > 0 {
				step := maxInt(1, m.timelineRows())
				m.cursor = minInt(len(m.entries)-1, m.cursor+step)
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "pgup", "ctrl+b":
			if len(m.entries) > 0 {
				step := maxInt(1, m.timelineRows())
				m.cursor = maxInt(0, m.cursor-step)
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "}":
			if m.mode == modeTimeline && len(m.entries) > 0 {
				m.cursor = m.jumpGroup(1)
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "{":
			if m.mode == modeTimeline && len(m.entries) > 0 {
				m.cursor = m.jumpGroup(-1)
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "down":
			if m.mode == modeAssign {
				if m.projectCursor < len(m.projects) {
					m.projectCursor++
				}
			} else if m.timelineView == timelineViewDay {
				m.moveSlot(15*time.Minute, 15*time.Minute)
			} else if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "j":
			if m.mode == modeAssign {
				if m.projectCursor < len(m.projects) {
					m.projectCursor++
				}
			} else if m.timelineView == timelineViewDay {
				m.jumpDayItem(1)
			} else if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "enter":
			if m.timelineView == timelineViewDay && m.dayFocusKind == "slot" {
				if idx := m.overlappingEntryIndexForSlot(); idx >= 0 {
					m.cursor = idx
					m.dayFocusKind = "entry"
					m.openEntryEditDialog(false)
					return m, cursorBlinkCmd()
				}
				m.openGapEntryDialog()
				return m, cursorBlinkCmd()
			}
			if m.timelineView == timelineViewDay && m.dayFocusKind == "gap" {
				m.openGapEntryDialog()
				return m, cursorBlinkCmd()
			}
			if len(m.entries) > 0 {
				m.openEntryEditDialog(false)
				return m, cursorBlinkCmd()
			}
			return m, nil
		case "a":
			if m.timelineView == timelineViewDay && m.dayFocusKind == "gap" {
				m.openGapEntryDialog()
				return m, cursorBlinkCmd()
			}
		case " ", "space":
			if m.mode == modeTimeline && len(m.entries) > 0 {
				entry := m.entries[m.cursor]
				m.selected[entry.ID] = !m.selected[entry.ID]
				if !m.selected[entry.ID] {
					delete(m.selected, entry.ID)
				}
			}
		case "p":
			if m.mode == modeTimeline && len(m.selected) > 0 && len(m.projects) > 0 {
				m.openProjectDialog(projectDialogAssign)
			}
		case "P":
			if len(m.entries) > 0 || len(m.projects) > 0 {
				m.openProjectDialog(projectDialogManage)
			}
		case "n":
			if m.timelineView == timelineViewDay {
				m.jumpToClosestSlot()
			} else if m.mode == modeTimeline && m.lastSearch != "" {
				m.jumpToSearchMatch(m.cursor, 1, false)
			}
		case "N":
			if m.mode == modeTimeline && m.lastSearch != "" {
				m.jumpToSearchMatch(m.cursor, -1, false)
			}
		case "r":
			if m.mode == modeTimeline && m.syncFn != nil && !m.syncing {
				m.syncing = true
				m.syncFrame = 0
				m.syncStatusErr = nil
				return m, tea.Batch(runSyncCmd(m.syncFn), syncPulseCmd(40*time.Millisecond))
			}
		case "d":
			if m.mode == modeTimeline && len(m.entries) > 0 {
				m.confirmDeleteID = m.entries[m.cursor].ID
				m.mode = modeDeleteConfirm
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureVisible()
	case syncPulseMsg:
		if !m.syncing {
			return m, nil
		}
		m.syncFrame++
		return m, syncPulseCmd(40 * time.Millisecond)
	case syncDoneMsg:
		m.syncing = false
		if msg.err != nil {
			m.syncStatusErr = msg.err
			return m, nil
		}
		now := time.Now().UTC()
		m.lastSyncedAt = &now
		m.syncStatusErr = nil
		m.restoreStateAfterReload()
	case cursorBlinkMsg:
		if m.mode != modeEntryEdit && m.mode != modeGapEntry {
			m.caretVisible = false
			return m, nil
		}
		m.caretVisible = !m.caretVisible
		return m, cursorBlinkCmd()
	}
	return m, nil
}

func (m AppModel) View() string {
	if m.quitting {
		return ""
	}
	styles := newStyles(m.width)
	var sections []string
	sections = append(sections, styles.header.Render(renderHeader(m.entries, m.width)))
	if m.err != nil {
		sections = append(sections, styles.error.Render("error: "+m.err.Error()))
	}
	var b strings.Builder
	b.WriteString(styles.title.Render("Timeline") + "\n")
	if len(m.entries) == 0 {
		b.WriteString(styles.muted.Render("no entries") + "\n")
	} else if m.timelineView == timelineViewDay {
		timelineModel := m
		if m.mode == modeTimeline {
			timelineModel.height = maxInt(12, m.height-inspectorHeight(m.height)-4)
		}
		b.WriteString(renderDayTimeline(timelineModel, styles))
	} else {
		cols := timelineColumns(m.width)
		header := renderTableHeader(cols, styles)
		b.WriteString(header + "\n")
		b.WriteString(styles.rule.Render(strings.Repeat("─", minInt(lipgloss.Width(stripANSI(header)), timelineWidth(m.width)))))
		b.WriteString("\n")
		rows := m.timelineRowsData()
		start, end := m.visibleRange(len(rows))
		for i := start; i < end; i++ {
			row := rows[i]
			if row.Header != "" {
				b.WriteString(styles.dateHeader.Render(renderDateHeader(row.Header, timelineWidth(m.width))) + "\n")
				continue
			}
			entry := row.Entry
			cursor := m.entryMarker(row.EntryIndex)
			project := "unassigned"
			if entry.ProjectName != "" {
				project = entry.ProjectName
			}
			desc := entry.ID
			if entry.Description != nil && *entry.Description != "" {
				desc = *entry.Description
			}
			line := renderEntryRow(cursor, entry, desc, project, cols, styles, row.EntryIndex == m.cursor, m.selected[entry.ID])
			b.WriteString(line + "\n")
		}
		if len(rows) > end {
			remaining := len(rows) - end
			b.WriteString(styles.muted.Render(truncateForWidth("... "+strconv.Itoa(remaining)+" more", timelineWidth(m.width))) + "\n")
		}
	}
	body := b.String()
	if m.mode == modeTimeline && m.timelineView == timelineViewDay {
		inspector := renderInspectorPane(m, styles, timelineWidth(m.width), inspectorHeight(m.height))
		if m.height > 0 {
			used := lineCount(renderHeader(m.entries, m.width)) + lineCount(body) + lineCount(inspector) + 1
			if m.err != nil {
				used++
			}
			remaining := m.height - used
			if remaining > 0 {
				body += strings.Repeat("\n", remaining)
			}
		}
		body += inspector
	}
	sections = append(sections, body)
	if m.mode == modeSearch {
		sections = append(sections, styles.title.Render("Search")+"\n"+styles.activePicker.Render("/"+m.searchQuery))
	}
	statusWidth := maxInt(0, timelineWidth(m.width)-3)
	statusText := padRight(renderStatusBar(m, statusWidth), statusWidth)
	sections = append(sections, styles.statusBar.Render(statusText))
	view := strings.Join(sections, "\n")
	if m.mode == modeAssign {
		return renderProjectDialog(m, styles, view)
	}
	if m.mode == modeGapEntry {
		return renderGapEntryDialog(m, styles, view)
	}
	if m.mode == modeEntryEdit {
		return renderEntryEditDialog(m, styles, view)
	}
	if m.mode == modeDeleteConfirm {
		return renderDeleteConfirmDialog(m, styles, view)
	}
	return view
}

type timelineColWidths struct {
	Cursor      int
	Time        int
	Description int
	Status      int
	Project     int
}

type dayTimelineCols struct {
	Lane  int
	Chart int
}

type dayWindow struct {
	start time.Time
	end   time.Time
}

type timelineBlock struct {
	entry model.TimeEntryDetail
	index int
	start time.Time
	end   time.Time
}

type dayGap struct {
	start time.Time
	end   time.Time
}

type dayTimelineItem struct {
	kind       string
	entryIndex int
	gapIndex   int
	start      time.Time
	end        time.Time
}

type dayOperatorLane struct {
	label  string
	blocks []timelineBlock
}

type dayTimelineRow struct {
	start time.Time
	end   time.Time
	label string
}

func (m *AppModel) ensureVisible() {
	rows := m.timelineRowsData()
	visible := m.timelineRows()
	if visible <= 0 {
		m.offset = 0
		return
	}
	selectedRow := m.selectedRowIndex(rows)
	if selectedRow < m.offset {
		m.offset = selectedRow
	}
	if selectedRow >= m.offset+visible {
		m.offset = selectedRow - visible + 1
	}
	maxOffset := maxInt(0, len(rows)-visible)
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
}

func (m AppModel) visibleRange(total int) (int, int) {
	visible := m.timelineRows()
	start := minInt(m.offset, maxInt(0, total))
	end := minInt(total, start+visible)
	return start, end
}

func (m AppModel) timelineRows() int {
	if m.height <= 0 {
		return len(m.entries)
	}
	rows := m.height - 7
	if rows < 1 {
		return 1
	}
	return rows
}

func timelineColumns(width int) timelineColWidths {
	if width <= 0 {
		width = 80
	}
	available := maxInt(35, width-4)
	cols := timelineColWidths{Cursor: 2, Time: 11, Status: 6, Project: 6, Description: 8}
	extra := available - (cols.Cursor + cols.Time + cols.Status + cols.Project + cols.Description)
	if extra > 0 {
		cols.Project += minInt(8, extra/3)
		extra -= minInt(8, extra/3)
		cols.Status += minInt(3, extra/4)
		extra -= minInt(3, extra/4)
		cols.Description += extra
	}
	return cols
}

func dayTimelineColumns(width int) dayTimelineCols {
	available := maxInt(40, timelineWidth(width)-2)
	lane := minInt(14, maxInt(8, available/6))
	chart := maxInt(24, available-lane-1)
	return dayTimelineCols{Lane: lane, Chart: chart}
}

func (m AppModel) timelineRowsData() []timelineRow {
	rows := make([]timelineRow, 0, len(m.entries)+8)
	lastDate := ""
	for i, entry := range m.entries {
		day := dayKey(entry.StartedAt)
		if day != lastDate {
			rows = append(rows, timelineRow{Header: day})
			lastDate = day
		}
		entryCopy := entry
		rows = append(rows, timelineRow{Entry: &entryCopy, EntryIndex: i})
	}
	return rows
}

func (m AppModel) selectedRowIndex(rows []timelineRow) int {
	for i, row := range rows {
		if row.Entry != nil && row.EntryIndex == m.cursor {
			return i
		}
	}
	return 0
}

func (m AppModel) jumpGroup(direction int) int {
	if len(m.entries) == 0 {
		return 0
	}
	currentDay := dayKey(m.entries[m.cursor].StartedAt)
	if direction > 0 {
		for i := m.cursor + 1; i < len(m.entries); i++ {
			day := dayKey(m.entries[i].StartedAt)
			if day != currentDay {
				return i
			}
		}
		return m.cursor
	}
	for i := m.cursor - 1; i >= 0; i-- {
		day := dayKey(m.entries[i].StartedAt)
		if day != currentDay {
			for i > 0 && dayKey(m.entries[i-1].StartedAt) == day {
				i--
			}
			return i
		}
	}
	return m.cursor
}

func (m *AppModel) focusCurrentEntryInDayView() {
	if len(m.entries) > 0 {
		m.dayDate = dayStart(m.entries[m.cursor].StartedAt)
	}
	m.dayFocusKind = "entry"
	m.dayGapFocus = 0
}

func (m *AppModel) setSlotFocus(ts time.Time, span time.Duration) {
	if span <= 0 {
		span = 15 * time.Minute
	}
	local := ts.In(time.Local)
	m.dayDate = dayStart(local)
	m.daySlotSpan = span
	m.daySlotStart = roundDownToStep(local, span)
	m.dayFocusKind = "slot"
	m.ensureSlotVisible()
}

func (m AppModel) displayedDay() time.Time {
	if !m.dayDate.IsZero() {
		return dayStart(m.dayDate)
	}
	if len(m.entries) > 0 {
		return dayStart(m.entries[m.cursor].StartedAt)
	}
	return dayStart(time.Now())
}

func (m *AppModel) shiftDisplayedDay(direction int) {
	if direction == 0 {
		return
	}
	next := m.displayedDay().AddDate(0, 0, direction)
	today := dayStart(time.Now())
	if next.After(today) {
		next = today
	}
	m.dayDate = next
	if m.dayWindowStart.IsZero() {
		m.dayWindowStart = defaultDayWindowStart(next)
	} else {
		m.dayWindowStart = clampDayWindowStart(time.Date(next.Year(), next.Month(), next.Day(), m.dayWindowStart.In(time.Local).Hour(), m.dayWindowStart.In(time.Local).Minute(), 0, 0, next.Location()), next)
	}
	if m.dayFocusKind == "slot" {
		if m.daySlotSpan == 0 {
			m.daySlotSpan = 15 * time.Minute
		}
		m.daySlotStart = roundDownToStep(time.Date(next.Year(), next.Month(), next.Day(), m.daySlotStart.In(time.Local).Hour(), m.daySlotStart.In(time.Local).Minute(), 0, 0, next.Location()), m.daySlotSpan)
		m.ensureSlotVisible()
		return
	}
	m.syncFocusForDisplayedDay()
}

func (m *AppModel) jumpToToday() {
	today := dayStart(time.Now())
	if m.displayedDay().Equal(today) {
		return
	}
	m.dayDate = today
	m.dayWindowStart = defaultDayWindowStart(today)
	m.setSlotFocus(time.Now(), 15*time.Minute)
}

func (m *AppModel) moveSlot(delta, span time.Duration) {
	if m.daySlotSpan == 0 {
		m.daySlotSpan = 15 * time.Minute
	}
	if span <= 0 {
		span = m.daySlotSpan
	}
	day := m.displayedDay()
	start := m.daySlotStart
	if start.IsZero() {
		start = roundDownToStep(time.Now().In(time.Local), span)
	}
	candidate := roundDownToStep(start.Add(delta), span)
	dayFloor := dayStart(day)
	dayMax := maxSlotStartForDay(day, span)
	if candidate.Before(dayFloor) {
		prevDay := day.AddDate(0, 0, -1)
		m.dayDate = prevDay
		candidate = maxSlotStartForDay(prevDay, span)
	} else if candidate.After(dayMax) {
		nextDay := day.AddDate(0, 0, 1)
		today := dayStart(time.Now())
		if nextDay.After(today) {
			candidate = dayMax
		} else {
			m.dayDate = nextDay
			candidate = clampSlotStartToDay(time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), candidate.In(time.Local).Hour(), candidate.In(time.Local).Minute(), 0, 0, nextDay.Location()), nextDay, span)
		}
	}
	m.daySlotSpan = span
	m.daySlotStart = candidate
	m.dayFocusKind = "slot"
	m.ensureSlotVisible()
}

func maxSlotStartForDay(day time.Time, span time.Duration) time.Time {
	day = dayStart(day)
	maxStart := day.Add(24 * time.Hour).Add(-span)
	if maxStart.Before(day) {
		return day
	}
	return roundDownToStep(maxStart, span)
}

func clampSlotStartToDay(start, day time.Time, span time.Duration) time.Time {
	day = dayStart(day)
	if start.Before(day) {
		return day
	}
	maxStart := maxSlotStartForDay(day, span)
	if start.After(maxStart) {
		return maxStart
	}
	return roundDownToStep(start, span)
}

func alignWindowStart(ts time.Time) time.Time {
	local := ts.In(time.Local)
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 0, 0, 0, local.Location())
}

func roundDownToStep(ts time.Time, step time.Duration) time.Time {
	if step <= 0 {
		return ts
	}
	base := ts.Unix()
	stepSecs := int64(step / time.Second)
	rounded := base - (base % stepSecs)
	return time.Unix(rounded, 0).In(ts.Location())
}

func (m *AppModel) syncFocusForDisplayedDay() {
	indices := m.dayEntryIndices(m.displayedDay().Format("2006-01-02"))
	if len(indices) == 0 {
		m.dayFocusKind = "gap"
		m.dayGapFocus = 0
		return
	}
	m.dayFocusKind = "entry"
	m.dayGapFocus = 0
	m.cursor = indices[0]
	m.ensureVisible()
}

func (m *AppModel) ensureSlotVisible() {
	if m.dayWindowStart.IsZero() {
		m.dayWindowStart = defaultDayWindowStart(m.displayedDay())
	}
	windowEnd := m.dayWindowStart.Add(10 * time.Hour)
	if m.daySlotStart.Before(m.dayWindowStart) {
		m.dayWindowStart = clampDayWindowStart(alignWindowStart(m.daySlotStart), m.displayedDay())
		return
	}
	if !m.daySlotStart.Before(windowEnd) {
		m.dayWindowStart = clampDayWindowStart(alignWindowStart(m.daySlotStart.Add(-9*time.Hour)), m.displayedDay())
	}
}

func (m *AppModel) jumpDayItem(direction int) {
	if direction == 0 {
		return
	}
	items := m.dayTimelineItems()
	if len(items) == 0 {
		return
	}
	if m.dayFocusKind == "slot" {
		target := m.daySlotStart
		if direction > 0 {
			for _, item := range items {
				if !item.start.Before(target) {
					m.focusDayItem(item)
					return
				}
			}
			return
		}
		for i := len(items) - 1; i >= 0; i-- {
			if !items[i].end.After(target) {
				m.focusDayItem(items[i])
				return
			}
		}
		return
	}
	current := m.currentDayItemPosition(items)
	if current == -1 {
		return
	}
	next := current + direction
	if next < 0 || next >= len(items) {
		return
	}
	m.focusDayItem(items[next])
	m.ensureVisible()
}

func (m *AppModel) jumpToClosestSlot() {
	if m.timelineView != timelineViewDay {
		return
	}
	if rng := m.selectedCreateRange(); rng != nil {
		m.setSlotFocus(rng.start, 15*time.Minute)
		return
	}
	if len(m.entries) > 0 {
		m.setSlotFocus(m.entries[m.cursor].StartedAt, 15*time.Minute)
	}
}

func (m AppModel) currentDayItemPosition(items []dayTimelineItem) int {
	for i, item := range items {
		if m.dayFocusKind == "gap" && item.kind == "gap" && item.gapIndex == m.dayGapFocus {
			return i
		}
		if m.dayFocusKind == "entry" && item.kind == "entry" && item.entryIndex == m.cursor {
			return i
		}
	}
	return -1
}

func (m *AppModel) focusDayItem(item dayTimelineItem) {
	if item.kind == "gap" {
		m.dayFocusKind = "gap"
		m.dayGapFocus = item.gapIndex
		return
	}
	m.dayFocusKind = "entry"
	m.cursor = item.entryIndex
}

func (m AppModel) dayEntryIndices(day string) []int {
	indices := make([]int, 0, len(m.entries))
	for i, entry := range m.entries {
		if dayKey(entry.StartedAt) == day {
			indices = append(indices, i)
		}
	}
	sort.SliceStable(indices, func(i, j int) bool {
		return m.entries[indices[i]].StartedAt.Before(m.entries[indices[j]].StartedAt)
	})
	return indices
}

func (m AppModel) dayTimelineItems() []dayTimelineItem {
	day := m.displayedDay().Format("2006-01-02")
	indices := m.dayEntryIndices(day)
	gaps := dayGapsForIndices(m.entries, indices, m.displayedDay())
	items := make([]dayTimelineItem, 0, len(indices)+len(gaps))
	for _, idx := range indices {
		entry := m.entries[idx]
		items = append(items, dayTimelineItem{kind: "entry", entryIndex: idx, start: entry.StartedAt, end: timelineBlockEnd(entry)})
	}
	for i, gap := range gaps {
		items = append(items, dayTimelineItem{kind: "gap", gapIndex: i, start: gap.start, end: gap.end})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].start.Equal(items[j].start) {
			if items[i].kind == items[j].kind {
				return items[i].end.Before(items[j].end)
			}
			return items[i].kind == "entry"
		}
		return items[i].start.Before(items[j].start)
	})
	return items
}

func (m AppModel) focusedGap() *dayGap {
	if m.dayFocusKind != "gap" {
		return nil
	}
	indices := m.dayEntryIndices(m.displayedDay().Format("2006-01-02"))
	gaps := dayGapsForIndices(m.entries, indices, m.displayedDay())
	if m.dayGapFocus < 0 || m.dayGapFocus >= len(gaps) {
		return nil
	}
	return &gaps[m.dayGapFocus]
}

func (m AppModel) selectedCreateRange() *dayGap {
	if gap := m.focusedGap(); gap != nil {
		return gap
	}
	if m.dayFocusKind == "slot" && !m.daySlotStart.IsZero() {
		gap := dayGap{start: m.daySlotStart, end: m.daySlotStart.Add(m.daySlotSpan)}
		return &gap
	}
	return nil
}

func (m AppModel) overlappingEntryIndexForSlot() int {
	if m.dayFocusKind != "slot" || m.daySlotStart.IsZero() {
		return -1
	}
	slotEnd := m.daySlotStart.Add(m.daySlotSpan)
	indices := m.dayEntryIndices(m.displayedDay().Format("2006-01-02"))
	for _, idx := range indices {
		entry := m.entries[idx]
		if rangesOverlap(m.daySlotStart, slotEnd, entry.StartedAt, timelineBlockEnd(entry)) {
			return idx
		}
	}
	return -1
}

func (m AppModel) entryMarker(index int) string {
	selected := m.selected[m.entries[index].ID]
	active := index == m.cursor && m.mode == modeTimeline
	switch {
	case active && selected:
		return ">*"
	case active:
		return "> "
	case selected:
		return " *"
	default:
		return "  "
	}
}

func (m AppModel) assignmentTargets(currentID string) []string {
	if len(m.selected) == 0 {
		return []string{currentID}
	}
	ids := make([]string, 0, len(m.selected))
	for _, entry := range m.entries {
		if m.selected[entry.ID] {
			ids = append(ids, entry.ID)
		}
	}
	if len(ids) == 0 {
		return []string{currentID}
	}
	return ids
}

func (m AppModel) assignEntries(entryIDs []string, projectID string) error {
	for _, entryID := range entryIDs {
		if err := m.store.AssignEntryToProject(m.ctx, entryID, projectID); err != nil {
			return err
		}
	}
	return nil
}

func (m AppModel) unassignEntries(entryIDs []string) error {
	for _, entryID := range entryIDs {
		if err := m.store.UnassignEntryProject(m.ctx, entryID); err != nil {
			return err
		}
	}
	return nil
}

func (m *AppModel) openProjectDialog(dialogMode projectDialogMode) {
	m.mode = modeAssign
	m.dialogMode = dialogMode
	m.projectInput = ""
	m.projectCursor = m.defaultProjectCursor(dialogMode)
	m.err = nil
	if dialogMode == projectDialogManage && len(m.projects) > 0 {
		m.projectCursor = 1
	}
}

func (m AppModel) defaultProjectCursor(dialogMode projectDialogMode) int {
	if dialogMode != projectDialogAssign || len(m.entries) == 0 {
		return 0
	}
	if len(m.selected) > 0 {
		return 0
	}
	entry := m.entries[m.cursor]
	if entry.ProjectID == nil {
		return 0
	}
	idx := m.projectIndex(*entry.ProjectID)
	if idx < 0 {
		return 0
	}
	return idx + 1
}

func nextEntryField(current string) string {
	switch current {
	case "description":
		return "project"
	case "project":
		return "start"
	case "start":
		return "end"
	default:
		return "description"
	}
}

func nextGapField(current string) string {
	switch current {
	case "description":
		return "project"
	case "project":
		return "start"
	case "start":
		return "end"
	default:
		return "description"
	}
}

func entryEditFieldIsText(field string) bool {
	return field == "description" || field == "start" || field == "end"
}

func gapEntryFieldIsText(field string) bool {
	return field == "description" || field == "start" || field == "end"
}

func (m *AppModel) appendEntryFieldInput(text string) {
	switch m.entryInputField {
	case "description":
		m.entryInput += text
	case "start":
		m.entryStartInput += text
	case "end":
		m.entryEndInput += text
	}
}

func (m *AppModel) backspaceEntryFieldInput() {
	switch m.entryInputField {
	case "description":
		if len(m.entryInput) > 0 {
			m.entryInput = m.entryInput[:len(m.entryInput)-1]
		}
	case "start":
		if len(m.entryStartInput) > 0 {
			m.entryStartInput = m.entryStartInput[:len(m.entryStartInput)-1]
		}
	case "end":
		if len(m.entryEndInput) > 0 {
			m.entryEndInput = m.entryEndInput[:len(m.entryEndInput)-1]
		}
	}
}

func (m *AppModel) clearEntryFieldInput() {
	switch m.entryInputField {
	case "description":
		m.entryInput = ""
	case "start":
		m.entryStartInput = ""
	case "end":
		m.entryEndInput = ""
	}
}

func (m *AppModel) appendGapFieldInput(text string) {
	switch m.gapInputField {
	case "description":
		m.gapInput += text
	case "start":
		m.gapStartInput += text
	case "end":
		m.gapEndInput += text
	}
}

func (m *AppModel) backspaceGapFieldInput() {
	switch m.gapInputField {
	case "description":
		if len(m.gapInput) > 0 {
			m.gapInput = m.gapInput[:len(m.gapInput)-1]
		}
	case "start":
		if len(m.gapStartInput) > 0 {
			m.gapStartInput = m.gapStartInput[:len(m.gapStartInput)-1]
		}
	case "end":
		if len(m.gapEndInput) > 0 {
			m.gapEndInput = m.gapEndInput[:len(m.gapEndInput)-1]
		}
	}
}

func (m *AppModel) clearGapFieldInput() {
	switch m.gapInputField {
	case "description":
		m.gapInput = ""
	case "start":
		m.gapStartInput = ""
	case "end":
		m.gapEndInput = ""
	}
}

func parseDialogRange(base time.Time, startText, endText string) (time.Time, time.Time, error) {
	loc := time.Local
	day := base.In(loc).Format("2006-01-02")
	startedAt, err := time.ParseInLocation("2006-01-02 15:04", day+" "+strings.TrimSpace(startText), loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start time")
	}
	endedAt, err := time.ParseInLocation("2006-01-02 15:04", day+" "+strings.TrimSpace(endText), loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end time")
	}
	if !endedAt.After(startedAt) {
		return time.Time{}, time.Time{}, fmt.Errorf("end must be after start")
	}
	return startedAt, endedAt, nil
}

func (m *AppModel) openGapEntryDialog() {
	if m.selectedCreateRange() == nil {
		return
	}
	rng := m.selectedCreateRange()
	m.mode = modeGapEntry
	m.gapInput = ""
	m.gapInputField = "description"
	m.gapStartInput = clock(rng.start)
	m.gapEndInput = clock(rng.end)
	m.gapProjectCursor = 0
	m.err = nil
	m.caretVisible = true
	if len(m.projects) > 0 {
		m.gapProjectCursor = 1
	}
}

func (m *AppModel) openEntryEditDialog(projectOnly bool) {
	if len(m.entries) == 0 {
		return
	}
	entry := m.entries[m.cursor]
	m.mode = modeEntryEdit
	m.entryProjectOnly = projectOnly
	m.entryInputField = "description"
	if projectOnly {
		m.entryInputField = "project"
	}
	m.entryInput = ""
	if entry.Description != nil {
		m.entryInput = *entry.Description
	}
	m.entryStartInput = clock(entry.StartedAt)
	if entry.EndedAt != nil {
		m.entryEndInput = clock(*entry.EndedAt)
	} else {
		m.entryEndInput = clock(entry.StartedAt.Add(time.Hour))
	}
	m.entryProjectCursor = 0
	if entry.ProjectID != nil {
		m.entryProjectCursor = m.projectIndex(*entry.ProjectID) + 1
	}
	m.err = nil
	m.caretVisible = true
}

func (m *AppModel) closeEntryEditDialog() {
	m.mode = modeTimeline
	m.entryInput = ""
	m.entryInputField = ""
	m.entryStartInput = ""
	m.entryEndInput = ""
	m.entryProjectCursor = 0
	m.entryProjectOnly = false
	m.caretVisible = false
}

func (m *AppModel) handleDeleteConfirmKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return tea.Quit
	case "y", "enter":
		if err := m.store.SoftDeleteEntry(m.ctx, m.confirmDeleteID); err != nil {
			m.err = err
			m.mode = modeTimeline
			m.confirmDeleteID = ""
			return nil
		}
		if err := m.reloadEntries(); err != nil {
			m.err = err
		}
		if m.cursor >= len(m.entries) && m.cursor > 0 {
			m.cursor = len(m.entries) - 1
		}
		m.mode = modeTimeline
		m.confirmDeleteID = ""
	case "n", "esc":
		m.mode = modeTimeline
		m.confirmDeleteID = ""
	}
	return nil
}

func (m *AppModel) handleEntryEditKey(msg tea.KeyMsg) tea.Cmd {
	if !m.entryProjectOnly && msg.Type == tea.KeyRunes && entryEditFieldIsText(m.entryInputField) {
		m.appendEntryFieldInput(string(msg.Runes))
		return nil
	}
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return tea.Quit
	case "esc":
		m.closeEntryEditDialog()
	case "tab":
		if !m.entryProjectOnly {
			m.entryInputField = nextEntryField(m.entryInputField)
		}
	case "up", "k":
		if m.entryProjectCursor > 0 {
			m.entryProjectCursor--
		}
	case "down", "j":
		if m.entryProjectCursor < len(m.projects) {
			m.entryProjectCursor++
		}
	case "backspace":
		if !m.entryProjectOnly {
			m.backspaceEntryFieldInput()
		}
	case " ", "space":
		if !m.entryProjectOnly && m.entryInputField == "description" {
			m.entryInput += " "
		}
	case "delete":
		if !m.entryProjectOnly {
			m.clearEntryFieldInput()
		}
	case "enter":
		m.saveEntryEdit()
	}
	return nil
}

func (m *AppModel) saveEntryEdit() {
	if len(m.entries) == 0 {
		m.closeEntryEditDialog()
		return
	}
	entry := m.entries[m.cursor]
	var projectID *string
	if m.entryProjectCursor > 0 && m.entryProjectCursor <= len(m.projects) {
		projectID = &m.projects[m.entryProjectCursor-1].ID
	}
	description := strings.TrimSpace(m.entryInput)
	if m.entryProjectOnly && entry.Description != nil {
		description = *entry.Description
	}
	startedAt, endedAt, err := parseDialogRange(entry.StartedAt, m.entryStartInput, m.entryEndInput)
	if err != nil {
		m.err = err
		return
	}
	if err := m.store.UpdateEntry(m.ctx, entry.ID, description, projectID, startedAt, endedAt); err != nil {
		m.err = err
		return
	}
	if err := m.reloadEntries(); err != nil {
		m.err = err
		return
	}
	m.focusEntryByID(entry.ID)
	m.closeEntryEditDialog()
}

func (m *AppModel) closeGapEntryDialog() {
	m.mode = modeTimeline
	m.gapInput = ""
	m.gapStartInput = ""
	m.gapEndInput = ""
	m.gapInputField = ""
	m.gapProjectCursor = 0
	m.caretVisible = false
}

func (m *AppModel) handleGapEntryKey(msg tea.KeyMsg) tea.Cmd {
	if msg.Type == tea.KeyRunes && gapEntryFieldIsText(m.gapInputField) {
		m.appendGapFieldInput(string(msg.Runes))
		return nil
	}
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return tea.Quit
	case "esc":
		m.closeGapEntryDialog()
	case "tab":
		m.gapInputField = nextGapField(m.gapInputField)
	case "up", "k":
		if m.gapProjectCursor > 1 {
			m.gapProjectCursor--
		}
	case "down", "j":
		if m.gapProjectCursor < len(m.projects) {
			m.gapProjectCursor++
		}
	case "backspace":
		m.backspaceGapFieldInput()
	case " ", "space":
		if m.gapInputField == "description" {
			m.gapInput += " "
		}
	case "delete":
		m.clearGapFieldInput()
	case "enter":
		m.createGapEntry()
	}
	return nil
}

func (m *AppModel) createGapEntry() {
	gap := m.selectedCreateRange()
	if gap == nil {
		m.err = fmt.Errorf("gap not found")
		m.closeGapEntryDialog()
		return
	}
	project := m.selectedGapProject()
	if project == nil {
		m.err = fmt.Errorf("project required")
		return
	}
	ident := project.Name
	if project.Code != nil && *project.Code != "" {
		ident = *project.Code
	}
	startedAt, endedAt, err := parseDialogRange(gap.start, m.gapStartInput, m.gapEndInput)
	if err != nil {
		m.err = err
		return
	}
	entry, err := m.store.CreateManualEntry(m.ctx, db.ManualEntryInput{
		ProjectIdent: ident,
		Description:  strings.TrimSpace(m.gapInput),
		StartedAt:    startedAt,
		EndedAt:      endedAt,
	})
	if err != nil {
		m.err = err
		return
	}
	if err := m.reloadEntries(); err != nil {
		m.err = err
		return
	}
	m.focusEntryByID(entry.ID)
	m.closeGapEntryDialog()
}

func (m *AppModel) selectedGapProject() *model.Project {
	if m.gapProjectCursor <= 0 || m.gapProjectCursor > len(m.projects) {
		return nil
	}
	return &m.projects[m.gapProjectCursor-1]
}

func (m *AppModel) closeProjectDialog() {
	m.mode = modeTimeline
	m.dialogMode = projectDialogAssign
	m.projectInput = ""
	m.projectCursor = 0
}

func (m *AppModel) handleProjectDialogKey(msg tea.KeyMsg) tea.Cmd {
	if m.dialogMode == projectDialogCreate {
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return tea.Quit
		case "esc":
			m.dialogMode = projectDialogManage
			m.projectInput = ""
		case "enter":
			name := strings.TrimSpace(m.projectInput)
			if name == "" {
				m.err = fmt.Errorf("project name required")
				return nil
			}
			project, err := m.store.CreateProject(m.ctx, db.ProjectCreateInput{Name: name, Currency: model.CurrencyEUR})
			if err != nil {
				m.err = err
				return nil
			}
			if err := m.reloadProjects(); err != nil {
				m.err = err
				return nil
			}
			m.dialogMode = projectDialogManage
			m.projectInput = ""
			m.projectCursor = m.projectIndex(project.ID) + 1
		case "backspace":
			if len(m.projectInput) > 0 {
				m.projectInput = m.projectInput[:len(m.projectInput)-1]
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.projectInput += string(msg.Runes)
			}
		}
		return nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return tea.Quit
	case "esc":
		m.closeProjectDialog()
	case "tab":
		if m.dialogMode == projectDialogAssign {
			m.dialogMode = projectDialogManage
			if m.projectCursor == 0 && len(m.projects) > 0 {
				m.projectCursor = 1
			}
		} else {
			m.dialogMode = projectDialogAssign
		}
	case "shift+tab":
		if m.dialogMode == projectDialogAssign {
			m.dialogMode = projectDialogManage
		} else {
			m.dialogMode = projectDialogAssign
		}
	case "up", "k":
		if m.projectCursor > m.projectCursorMin() {
			m.projectCursor--
		}
	case "down", "j":
		if m.projectCursor < len(m.projects) {
			m.projectCursor++
		}
	case "home":
		m.projectCursor = m.projectCursorMin()
	case "end":
		if len(m.projects) > 0 {
			m.projectCursor = len(m.projects)
		}
	case "pgdown", "ctrl+f":
		step := maxInt(1, m.timelineRows())
		m.projectCursor = minInt(len(m.projects), m.projectCursor+step)
		if m.projectCursor < m.projectCursorMin() {
			m.projectCursor = m.projectCursorMin()
		}
	case "pgup", "ctrl+b":
		step := maxInt(1, m.timelineRows())
		m.projectCursor = maxInt(m.projectCursorMin(), m.projectCursor-step)
	case "a":
		if m.dialogMode == projectDialogManage {
			m.dialogMode = projectDialogCreate
			m.projectInput = ""
		}
	case "b":
		if m.dialogMode == projectDialogManage {
			m.toggleProjectBillable()
		}
	case "c":
		if m.dialogMode == projectDialogManage {
			m.cycleProjectColor()
		}
	case "C":
		if m.dialogMode == projectDialogManage {
			m.randomizeProjectColor()
		}
	case "x":
		if m.dialogMode == projectDialogManage {
			m.archiveSelectedProject()
		}
	case "enter":
		if m.dialogMode == projectDialogAssign {
			m.confirmProjectAssignment()
		}
	}
	return nil
}

func (m AppModel) projectCursorMin() int {
	if m.dialogMode == projectDialogManage {
		if len(m.projects) == 0 {
			return 0
		}
		return 1
	}
	return 0
}

func (m *AppModel) confirmProjectAssignment() {
	if len(m.entries) == 0 {
		m.closeProjectDialog()
		return
	}
	entry := m.entries[m.cursor]
	targetIDs := m.assignmentTargets(entry.ID)
	var err error
	if m.projectCursor == 0 {
		err = m.unassignEntries(targetIDs)
	} else if m.projectCursor-1 < len(m.projects) {
		err = m.assignEntries(targetIDs, m.projects[m.projectCursor-1].ID)
	}
	if err != nil {
		m.err = err
		m.closeProjectDialog()
		return
	}
	if err := m.reloadEntries(); err != nil {
		m.err = err
		m.closeProjectDialog()
		return
	}
	m.selected = map[string]bool{}
	m.closeProjectDialog()
}

func (m *AppModel) toggleProjectBillable() {
	project := m.selectedProject()
	if project == nil {
		return
	}
	updated, err := m.store.UpdateProjectBillableDefaultByID(m.ctx, project.ID, !project.BillableDefault)
	if err != nil {
		m.err = err
		return
	}
	if err := m.reloadProjects(); err != nil {
		m.err = err
		return
	}
	m.projectCursor = m.projectIndex(updated.ID) + 1
}

func (m *AppModel) archiveSelectedProject() {
	project := m.selectedProject()
	if project == nil {
		return
	}
	if err := m.store.ArchiveProjectByID(m.ctx, project.ID); err != nil {
		m.err = err
		return
	}
	if err := m.reloadProjects(); err != nil {
		m.err = err
		return
	}
	if len(m.projects) == 0 {
		m.projectCursor = 0
		return
	}
	m.projectCursor = minInt(m.projectCursor, len(m.projects))
	if m.projectCursor == 0 {
		m.projectCursor = 1
	}
}

func (m *AppModel) cycleProjectColor() {
	project := m.selectedProject()
	if project == nil {
		return
	}
	colors := db.DefaultProjectColors()
	if len(colors) == 0 {
		return
	}
	current := ""
	if project.Color != nil {
		current = strings.ToLower(*project.Color)
	}
	next := colors[0]
	for i, color := range colors {
		if strings.ToLower(color) == current {
			next = colors[(i+1)%len(colors)]
			break
		}
	}
	updated, err := m.store.UpdateProjectColorByID(m.ctx, project.ID, next)
	if err != nil {
		m.err = err
		return
	}
	if err := m.reloadProjects(); err != nil {
		m.err = err
		return
	}
	m.projectCursor = m.projectIndex(updated.ID) + 1
}

func (m *AppModel) randomizeProjectColor() {
	project := m.selectedProject()
	if project == nil {
		return
	}
	colors := db.DefaultProjectColors()
	if len(colors) == 0 {
		return
	}
	current := ""
	if project.Color != nil {
		current = strings.ToLower(*project.Color)
	}
	choice := colors[int(time.Now().UnixNano())%len(colors)]
	if len(colors) > 1 && strings.ToLower(choice) == current {
		choice = colors[(int(time.Now().UnixNano())+1)%len(colors)]
	}
	updated, err := m.store.UpdateProjectColorByID(m.ctx, project.ID, choice)
	if err != nil {
		m.err = err
		return
	}
	if err := m.reloadProjects(); err != nil {
		m.err = err
		return
	}
	m.projectCursor = m.projectIndex(updated.ID) + 1
}

func (m *AppModel) selectedProject() *model.Project {
	if m.projectCursor <= 0 || m.projectCursor > len(m.projects) {
		return nil
	}
	return &m.projects[m.projectCursor-1]
}

func (m *AppModel) reloadProjects() error {
	projects, err := m.store.ListProjects(m.ctx)
	if err != nil {
		return err
	}
	m.projects = projects
	return nil
}

func (m *AppModel) reloadEntries() error {
	entries, err := m.store.ListEntries(m.ctx)
	if err != nil {
		return err
	}
	m.allEntries = sortEntries(entries)
	m.applySourceFilter()
	return nil
}

func (m *AppModel) restoreStateAfterReload() {
	prevDay := m.displayedDay()
	prevKind := m.dayFocusKind
	prevGap := m.dayGapFocus
	prevEntryID := ""
	if len(m.entries) > 0 && m.cursor >= 0 && m.cursor < len(m.entries) {
		prevEntryID = m.entries[m.cursor].ID
	}
	if err := m.reloadEntries(); err != nil {
		m.syncStatusErr = err
		return
	}
	m.dayDate = prevDay
	if prevKind == "gap" {
		m.dayFocusKind = "gap"
		m.dayGapFocus = prevGap
		if m.focusedGap() == nil {
			m.syncFocusForDisplayedDay()
		}
		return
	}
	if prevEntryID != "" {
		for i, entry := range m.entries {
			if entry.ID == prevEntryID {
				m.cursor = i
				m.dayFocusKind = "entry"
				m.dayGapFocus = 0
				m.ensureVisible()
				return
			}
		}
	}
	m.syncFocusForDisplayedDay()
}

func syncPulseCmd(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg { return syncPulseMsg{} })
}

func cursorBlinkCmd() tea.Cmd {
	return tea.Tick(530*time.Millisecond, func(time.Time) tea.Msg { return cursorBlinkMsg{} })
}

func runSyncCmd(syncFn func() error) tea.Cmd {
	return func() tea.Msg {
		return syncDoneMsg{err: syncFn()}
	}
}

func (m *AppModel) focusEntryByID(id string) {
	for i, entry := range m.entries {
		if entry.ID == id {
			m.cursor = i
			m.focusCurrentEntryInDayView()
			m.ensureVisible()
			return
		}
	}
}

func (m AppModel) projectIndex(id string) int {
	for i, project := range m.projects {
		if project.ID == id {
			return i
		}
	}
	return -1
}

func sortEntries(entries []model.TimeEntryDetail) []model.TimeEntryDetail {
	sorted := append([]model.TimeEntryDetail(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].StartedAt.After(sorted[j].StartedAt)
	})
	return sorted
}

type tuiStyles struct {
	header        lipgloss.Style
	title         lipgloss.Style
	error         lipgloss.Style
	rule          lipgloss.Style
	dateHeader    lipgloss.Style
	muted         lipgloss.Style
	statusBar     lipgloss.Style
	tableHeader   lipgloss.Style
	baseRow       lipgloss.Style
	activeRow     lipgloss.Style
	selectedRow   lipgloss.Style
	activeSelRow  lipgloss.Style
	draft         lipgloss.Style
	confirmed     lipgloss.Style
	projectPicker lipgloss.Style
	activePicker  lipgloss.Style
	dialogBox     lipgloss.Style
	inspectorBox  lipgloss.Style
	inspectorTab  lipgloss.Style
	activeTab     lipgloss.Style
}

func newStyles(width int) tuiStyles {
	barWidth := timelineWidth(width)
	return tuiStyles{
		header:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Width(barWidth),
		title:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")),
		error:         lipgloss.NewStyle().Foreground(lipgloss.Color("204")).Bold(true),
		rule:          lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		dateHeader:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")),
		muted:         lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		statusBar:     lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63")),
		tableHeader:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")),
		baseRow:       lipgloss.NewStyle(),
		activeRow:     lipgloss.NewStyle().Background(lipgloss.Color("236")),
		selectedRow:   lipgloss.NewStyle().Background(lipgloss.Color("60")),
		activeSelRow:  lipgloss.NewStyle().Background(lipgloss.Color("99")).Bold(true),
		draft:         lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
		confirmed:     lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
		projectPicker: lipgloss.NewStyle(),
		activePicker:  lipgloss.NewStyle().Background(lipgloss.Color("99")).Foreground(lipgloss.Color("230")).Bold(true),
		dialogBox:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("99")).Background(lipgloss.Color("235")).Padding(1, 2),
		inspectorBox:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1),
		inspectorTab:  lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1),
		activeTab:     lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("99")).Bold(true).Padding(0, 1),
	}
}

func renderHeader(entries []model.TimeEntryDetail, width int) string {
	rangeText := currentRange(entries)
	left := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render("hrs")
	right := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(rangeText)
	spacer := maxInt(1, timelineWidth(width)-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", spacer) + right
}

func currentRange(entries []model.TimeEntryDetail) string {
	if len(entries) == 0 {
		return time.Now().In(time.Local).Format("2006-01-02")
	}
	newest := dayKey(entries[0].StartedAt)
	oldest := dayKey(entries[len(entries)-1].StartedAt)
	if newest == oldest {
		return newest
	}
	return oldest + " to " + newest
}

func lineCount(text string) int {
	trimmed := strings.TrimRight(text, "\n")
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

func renderTableHeader(cols timelineColWidths, styles tuiStyles) string {
	line := fmt.Sprintf("%s %s %s %s %s",
		padRight(" ", cols.Cursor),
		padRight("Time", cols.Time),
		padRight("Description", cols.Description),
		padRight("Status", cols.Status),
		padRight("Project", cols.Project),
	)
	return styles.tableHeader.Render(line)
}

func renderDateHeader(date string, width int) string {
	label := "── " + date + " ──"
	if lipgloss.Width(label) >= width {
		return truncateForWidth(label, width)
	}
	return label + strings.Repeat("─", width-lipgloss.Width(label))
}

func renderEntryRow(cursor string, entry *model.TimeEntryDetail, desc, project string, cols timelineColWidths, styles tuiStyles, active, selected bool) string {
	statusText := truncateForWidth(string(entry.Status), cols.Status)
	statusCell := styles.draft
	if entry.Status == model.StatusConfirmed {
		statusCell = styles.confirmed
	}
	projectCell := lipgloss.NewStyle()
	if entry.ProjectColor != nil && *entry.ProjectColor != "" {
		projectCell = projectCell.Foreground(lipgloss.Color(*entry.ProjectColor)).Bold(true)
	}
	line := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(cols.Cursor).Render(cursor),
		lipgloss.NewStyle().Width(1).Render(" "),
		lipgloss.NewStyle().Width(cols.Time).Render(padRight(formatRange(entry.StartedAt, entry.EndedAt), cols.Time)),
		lipgloss.NewStyle().Width(1).Render(" "),
		lipgloss.NewStyle().Width(cols.Description).Render(padRight(truncateForWidth(desc, cols.Description), cols.Description)),
		lipgloss.NewStyle().Width(1).Render(" "),
		statusCell.Width(cols.Status).Render(padRight(statusText, cols.Status)),
		lipgloss.NewStyle().Width(1).Render(" "),
		projectCell.Width(cols.Project).Render(padRight(truncateForWidth(project, cols.Project), cols.Project)),
	)
	rowStyle := styles.baseRow
	switch {
	case active && selected:
		rowStyle = styles.activeSelRow
	case active:
		rowStyle = styles.activeRow
	case selected:
		rowStyle = styles.selectedRow
	}
	return rowStyle.Render(line)
}

func renderDayTimeline(m AppModel, styles tuiStyles) string {
	selectedDay := m.displayedDay().Format("2006-01-02")
	selectedWeekday := m.displayedDay().Format("Monday")
	indices := m.dayEntryIndices(selectedDay)
	dayEntries := dayEntriesForDate(m.entries, selectedDay)
	gaps := dayGapsForIndices(m.entries, indices, m.displayedDay())
	window := dayTimelineWindow(dayEntries, m.displayedDay(), m.dayWindowStart)
	operatorLanes := buildOperatorLanes(m.entries, indices)
	rows := dayTimelineRows(window, m.height)
	chartCols := verticalTimelineColumns(m.width, operatorLanes)

	var b strings.Builder
	b.WriteString(styles.dateHeader.Render(renderDateHeader(selectedWeekday+" "+selectedDay, timelineWidth(m.width))) + "\n")
	b.WriteString(styles.muted.Render(fmt.Sprintf("%d entries | %d lanes | %d gaps | %s-%s", len(dayEntries), len(operatorLanes), len(gaps), clock(window.start), clock(window.end))) + "\n")
	b.WriteString(renderVerticalTimelineHeader(chartCols, operatorLanes) + "\n")
	b.WriteString(styles.rule.Render(strings.Repeat("-", timelineWidth(m.width))) + "\n")
	for _, row := range rows {
		b.WriteString(renderVerticalTimelineRow(m, row, chartCols, operatorLanes, styles) + "\n")
	}
	if m.dayFocusKind == "slot" && !m.daySlotStart.IsZero() {
		b.WriteString(styles.muted.Render(fmt.Sprintf("%s | slot %s-%s | enter create or edit overlap", selectedWeekday, clock(m.daySlotStart), clock(m.daySlotStart.Add(m.daySlotSpan)))))
		return b.String()
	}
	if gap := m.focusedGap(); gap != nil {
		b.WriteString(styles.muted.Render(fmt.Sprintf("%s | focus gap %s-%s | %s | enter add entry", selectedWeekday, clock(gap.start), clock(gap.end), formatGapDuration(*gap))))
		return b.String()
	}
	if len(dayEntries) == 0 {
		b.WriteString(styles.muted.Render(fmt.Sprintf("%s | focus empty day | left/right change day | t today", selectedWeekday)))
		return b.String()
	}
	selected := m.entries[m.cursor]
	project := "unassigned"
	if selected.ProjectName != "" {
		project = selected.ProjectName
	}
	label := timelineBlockLabel(selected)
	b.WriteString(styles.muted.Render(fmt.Sprintf("%s | focus %s | %s | %s | %s", selectedWeekday, formatRange(selected.StartedAt, selected.EndedAt), selected.Operator, truncateForWidth(label, 32), truncateForWidth(project, 20))))
	return b.String()
}

func inspectorHeight(height int) int {
	if height <= 0 {
		return 10
	}
	return minInt(12, maxInt(8, height/3))
}

func renderInspectorPane(m AppModel, styles tuiStyles, width int, height int) string {
	innerWidth := maxInt(20, width-4)
	tabs := renderInspectorTabs(m, styles)
	body := renderInspectorBody(m, innerWidth, maxInt(1, height-3))
	content := tabs + "\n" + body
	return styles.inspectorBox.Width(maxInt(20, width-2)).Render(content)
}

func renderInspectorTabs(m AppModel, styles tuiStyles) string {
	tabs := []struct {
		label string
		id    inspectorTab
	}{
		{label: "Overview", id: inspectorOverview},
		{label: "Session", id: inspectorSession},
		{label: "Actions", id: inspectorActions},
	}
	parts := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		style := styles.inspectorTab
		if m.inspectorTab == tab.id {
			style = styles.activeTab
		}
		parts = append(parts, style.Render(tab.label))
	}
	return strings.Join(parts, " ")
}

func renderInspectorBody(m AppModel, width int, height int) string {
	lines := inspectorLines(m)
	for i, line := range lines {
		lines[i] = truncateForWidth(line, width)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func inspectorLines(m AppModel) []string {
	switch m.inspectorTab {
	case inspectorSession:
		return sessionInspectorLines(m)
	case inspectorActions:
		return actionInspectorLines(m)
	default:
		return overviewInspectorLines(m)
	}
}

func overviewInspectorLines(m AppModel) []string {
	if m.dayFocusKind == "slot" && !m.daySlotStart.IsZero() {
		return []string{
			"Time slot",
			fmt.Sprintf("Range: %s-%s", clock(m.daySlotStart), clock(m.daySlotStart.Add(m.daySlotSpan))),
			fmt.Sprintf("Size: %s", formatGapDuration(dayGap{start: m.daySlotStart, end: m.daySlotStart.Add(m.daySlotSpan)})),
			"Action: Enter creates entry or edits overlap",
		}
	}
	if gap := m.focusedGap(); gap != nil {
		return []string{
			"Gap",
			fmt.Sprintf("Range: %s-%s", clock(gap.start), clock(gap.end)),
			"Duration: " + formatGapDuration(*gap),
			"Action: Enter adds manual entry",
		}
	}
	if len(m.entries) == 0 {
		return []string{"No entry selected"}
	}
	entry := m.entries[m.cursor]
	project := "unassigned"
	if entry.ProjectName != "" {
		project = entry.ProjectName
	}
	label := entry.ID
	if entry.Description != nil && *entry.Description != "" {
		label = *entry.Description
	}
	lines := []string{
		label,
		fmt.Sprintf("Project: %s", project),
		fmt.Sprintf("Time: %s", formatRange(entry.StartedAt, entry.EndedAt)),
		fmt.Sprintf("Operator: %s", entry.Operator),
		fmt.Sprintf("Status: %s", entry.Status),
		fmt.Sprintf("Billable: %t", entry.Billable),
	}
	if entry.Cwd != nil && *entry.Cwd != "" {
		lines = append(lines, "CWD: "+*entry.Cwd)
	}
	if entry.GitBranch != nil && *entry.GitBranch != "" {
		lines = append(lines, "Branch: "+*entry.GitBranch)
	}
	return lines
}

func sessionInspectorLines(m AppModel) []string {
	if m.dayFocusKind == "slot" {
		return []string{"No session", "Time slots are creation targets, not sessions."}
	}
	if m.dayFocusKind == "gap" {
		return []string{"No session", "Gaps do not have source session detail."}
	}
	if len(m.entries) == 0 {
		return []string{"No session selected"}
	}
	entry := m.entries[m.cursor]
	if entry.Operator == "human" {
		return []string{"Manual entry", "No agent session attached."}
	}
	lines := []string{fmt.Sprintf("Source: %s", entry.Operator)}
	if details, err := intsync.LoadSessionDetail(entry); err == nil {
		lines = append(lines, details...)
	} else {
		lines = append(lines, "Detail error: "+err.Error())
	}
	lines = append(lines, fmt.Sprintf("Range: %s", formatRange(entry.StartedAt, entry.EndedAt)))
	return lines
}

func actionInspectorLines(m AppModel) []string {
	if m.dayFocusKind == "slot" {
		return []string{
			"Enter: create entry or edit overlap",
			"Up/Down: move 15m",
			"Shift+Up/Down: move 1h",
			"j/k: prev/next item",
			"Left/Right: prev/next day",
		}
	}
	if m.dayFocusKind == "gap" {
		return []string{
			"Enter: add manual entry",
			"Tab: next inspector tab",
			"Left/Right: prev/next day",
		}
	}
	lines := []string{
		"Enter: edit description + project",
		"Space: select entry",
		"Tab: next inspector tab",
		"Left/Right: prev/next day",
		"P: manage projects",
		"R: sync now",
	}
	if len(m.selected) > 0 {
		lines = append(lines, "p: assign selected entries")
	}
	return lines
}

func metadataString(md map[string]any, key string) string {
	if md == nil {
		return ""
	}
	v, ok := md[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case float64:
		return strconv.Itoa(int(val))
	case int:
		return strconv.Itoa(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func dayTimelineRows(window dayWindow, height int) []dayTimelineRow {
	available := maxInt(10, height-6)
	base := available / 10
	extra := available % 10
	if base < 1 {
		base = 1
		extra = 0
	}
	rows := make([]dayTimelineRow, 0, available)
	hourStart := window.start
	for hour := 0; hour < 10; hour++ {
		rowsThisHour := base
		if hour < extra {
			rowsThisHour++
		}
		step := time.Hour / time.Duration(rowsThisHour)
		for i := 0; i < rowsThisHour; i++ {
			start := hourStart.Add(time.Duration(i) * step)
			end := start.Add(step)
			if i == rowsThisHour-1 {
				end = hourStart.Add(time.Hour)
			}
			label := ""
			if i == 0 {
				label = clock(hourStart)
			}
			rows = append(rows, dayTimelineRow{start: start, end: end, label: label})
		}
		hourStart = hourStart.Add(time.Hour)
	}
	return rows
}

func verticalTimelineColumns(width int, lanes []dayOperatorLane) []int {
	cols := make([]int, len(lanes))
	for i, lane := range lanes {
		maxLabel := lipgloss.Width(lane.label)
		for _, block := range lane.blocks {
			maxLabel = maxInt(maxLabel, lipgloss.Width(timelineBlockLabel(block.entry)))
		}
		cols[i] = minInt(36, maxInt(12, maxLabel+2))
	}
	return cols
}

func renderVerticalTimelineHeader(colWidths []int, lanes []dayOperatorLane) string {
	parts := []string{padRight("time", 5)}
	for i, lane := range lanes {
		parts = append(parts, " ", padRight(truncateForWidth(lane.label, colWidths[i]), colWidths[i]))
	}
	return strings.Join(parts, "")
}

func renderVerticalTimelineRow(m AppModel, row dayTimelineRow, colWidths []int, lanes []dayOperatorLane, styles tuiStyles) string {
	parts := []string{renderVerticalTimeCell(m, row, styles)}
	for i, lane := range lanes {
		parts = append(parts, " ", renderVerticalLaneCell(row.start, row.end, lane.blocks, m, colWidths[i], styles))
	}
	return strings.Join(parts, "")
}

func renderVerticalTimeCell(m AppModel, row dayTimelineRow, styles tuiStyles) string {
	label := padRight(row.label, 5)
	if m.dayFocusKind == "slot" && !m.daySlotStart.IsZero() && rangesOverlap(row.start, row.end, m.daySlotStart, m.daySlotStart.Add(m.daySlotSpan)) {
		return styles.activePicker.Width(5).Render(label)
	}
	return label
}

func renderVerticalLaneCell(slotStart, slotEnd time.Time, blocks []timelineBlock, m AppModel, width int, styles tuiStyles) string {
	for _, block := range blocks {
		if !rangesOverlap(slotStart, slotEnd, block.start, block.end) {
			continue
		}
		focused := (m.dayFocusKind == "entry" && block.index == m.cursor) || (m.dayFocusKind == "slot" && slotOverlapsLaneBlock(m, block, blocks))
		cellStyle := styles.confirmed
		if block.entry.Status == model.StatusDraft {
			cellStyle = styles.draft
		}
		if block.entry.ProjectColor != nil && *block.entry.ProjectColor != "" && !focused {
			cellStyle = cellStyle.Foreground(lipgloss.Color(*block.entry.ProjectColor))
		}
		return renderVerticalEntryCell(slotStart, slotEnd, block.start, block.end, focused, width, timelineBlockLabel(block.entry), cellStyle, styles)
	}
	return padRight("", width)
}

func slotOverlapsLaneBlock(m AppModel, block timelineBlock, laneBlocks []timelineBlock) bool {
	if m.dayFocusKind != "slot" || m.daySlotStart.IsZero() || m.daySlotSpan <= 0 {
		return false
	}
	slotEnd := m.daySlotStart.Add(m.daySlotSpan)
	if !rangesOverlap(m.daySlotStart, slotEnd, block.start, block.end) {
		return false
	}
	for _, b := range laneBlocks {
		if rangesOverlap(m.daySlotStart, slotEnd, b.start, b.end) {
			return b.index == block.index
		}
	}
	return false
}

func renderVerticalEntryCell(slotStart, slotEnd, itemStart, itemEnd time.Time, focused bool, width int, label string, baseStyle lipgloss.Style, styles tuiStyles) string {
	if focused {
		return renderVerticalRangeCell(slotStart, slotEnd, itemStart, itemEnd, true, width, label, baseStyle, styles)
	}
	text := outlinedBlockCell(slotStart, slotEnd, itemStart, itemEnd, width, label)
	if strings.TrimSpace(text) == "" {
		return padRight("", width)
	}
	return baseStyle.Render(text)
}

func renderVerticalRangeCell(slotStart, slotEnd, itemStart, itemEnd time.Time, focused bool, width int, label string, baseStyle lipgloss.Style, styles tuiStyles) string {
	text := " "
	starts := !itemStart.Before(slotStart) && itemStart.Before(slotEnd)
	ends := itemEnd.After(slotStart) && !itemEnd.After(slotEnd)
	midpoint := itemStart.Add(itemEnd.Sub(itemStart) / 2)
	containsMid := !midpoint.Before(slotStart) && midpoint.Before(slotEnd)
	if starts && ends {
		text = centeredBlockLabel(label, width)
	} else if containsMid {
		text = centeredBlockLabel(label, width)
	} else {
		text = strings.Repeat(" ", width)
	}
	style := baseStyle
	if focused {
		style = styles.activePicker
	}
	return style.Render(padRight(truncateForWidth(text, width), width))
}

func outlinedBlockCell(slotStart, slotEnd, itemStart, itemEnd time.Time, width int, label string) string {
	if width <= 1 {
		return "│"
	}
	starts := !itemStart.Before(slotStart) && itemStart.Before(slotEnd)
	ends := itemEnd.After(slotStart) && !itemEnd.After(slotEnd)
	midpoint := itemStart.Add(itemEnd.Sub(itemStart) / 2)
	containsMid := !midpoint.Before(slotStart) && midpoint.Before(slotEnd)
	innerWidth := maxInt(0, width-2)
	fill := strings.Repeat("─", innerWidth)
	space := strings.Repeat(" ", innerWidth)
	if starts && ends {
		return "┌" + padRight(truncateForWidth(label, innerWidth), innerWidth) + "┐"
	}
	if starts {
		return "┌" + fill + "┐"
	}
	if ends {
		return "└" + fill + "┘"
	}
	if containsMid {
		return "│" + padRight(truncateForWidth(label, innerWidth), innerWidth) + "│"
	}
	return "│" + space + "│"
}

func centeredBlockLabel(label string, width int) string {
	if width <= 1 {
		return ""
	}
	trimmed := truncateForWidth(label, width-2)
	if strings.TrimSpace(trimmed) == "" {
		return strings.Repeat(" ", width)
	}
	text := " " + trimmed
	return padRight(text, width)
}

func rangesOverlap(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && aEnd.After(bStart)
}

func dayEntriesForDate(entries []model.TimeEntryDetail, date string) []model.TimeEntryDetail {
	filtered := make([]model.TimeEntryDetail, 0, len(entries))
	for _, entry := range entries {
		if dayKey(entry.StartedAt) == date {
			filtered = append(filtered, entry)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].StartedAt.Before(filtered[j].StartedAt)
	})
	return filtered
}

func dayTimelineWindow(entries []model.TimeEntryDetail, day time.Time, startOverride time.Time) dayWindow {
	day = dayStart(day)
	if !startOverride.IsZero() {
		start := clampDayWindowStart(startOverride, day)
		return dayWindow{start: start, end: start.Add(10 * time.Hour)}
	}
	if len(entries) == 0 {
		start := time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, day.Location())
		return dayWindow{start: start, end: start.Add(10 * time.Hour)}
	}
	start := entries[0].StartedAt.In(time.Local)
	end := timelineBlockEnd(entries[0]).In(time.Local)
	for _, entry := range entries[1:] {
		entryStart := entry.StartedAt.In(time.Local)
		if entryStart.Before(start) {
			start = entryStart
		}
		entryEnd := timelineBlockEnd(entry).In(time.Local)
		if entryEnd.After(end) {
			end = entryEnd
		}
	}
	startHour := maxInt(0, start.Hour()-1)
	endHour := minInt(24, end.Hour()+1)
	if end.Minute() > 0 || end.Second() > 0 || end.Nanosecond() > 0 {
		endHour = minInt(24, endHour+1)
	}
	if endHour-startHour <= 10 {
		windowStart := time.Date(day.Year(), day.Month(), day.Day(), startHour, 0, 0, 0, day.Location())
		windowEnd := windowStart.Add(10 * time.Hour)
		dayEnd := day.Add(24 * time.Hour)
		if windowEnd.After(dayEnd) {
			windowEnd = dayEnd
			windowStart = windowEnd.Add(-10 * time.Hour)
			if windowStart.Before(day) {
				windowStart = day
			}
		}
		return dayWindow{start: windowStart, end: windowEnd}
	}
	windowEnd := time.Date(day.Year(), day.Month(), day.Day(), endHour, 0, 0, 0, day.Location())
	windowStart := windowEnd.Add(-10 * time.Hour)
	if windowStart.Before(day) {
		windowStart = day
		windowEnd = windowStart.Add(10 * time.Hour)
	}
	return dayWindow{start: windowStart, end: windowEnd}
}

func defaultDayWindowStart(day time.Time) time.Time {
	day = dayStart(day)
	now := time.Now().In(time.Local)
	if day.Equal(dayStart(now)) {
		start := time.Date(day.Year(), day.Month(), day.Day(), now.Hour()-4, 0, 0, 0, day.Location())
		return clampDayWindowStart(start, day)
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, day.Location())
	return clampDayWindowStart(start, day)
}

func clampDayWindowStart(start, day time.Time) time.Time {
	day = dayStart(day)
	start = start.In(day.Location())
	minStart := day
	maxStart := day.Add(14 * time.Hour)
	if start.Before(minStart) {
		return minStart
	}
	if start.After(maxStart) {
		return maxStart
	}
	return time.Date(day.Year(), day.Month(), day.Day(), start.Hour(), start.Minute(), 0, 0, day.Location())
}

func buildOperatorLanes(entries []model.TimeEntryDetail, indices []int) []dayOperatorLane {
	groups := map[string][]timelineBlock{}
	order := make([]string, 0, 4)
	for _, idx := range indices {
		entry := entries[idx]
		label := entry.Operator
		if label == "" {
			label = "unknown"
		}
		if _, ok := groups[label]; !ok {
			order = append(order, label)
		}
		groups[label] = append(groups[label], timelineBlock{entry: entry, index: idx, start: entry.StartedAt, end: timelineBlockEnd(entry)})
	}
	lanes := make([]dayOperatorLane, 0, len(indices))
	for _, label := range order {
		blocks := groups[label]
		sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].start.Before(blocks[j].start) })
		sublanes := assignBlockLanes(blocks)
		for i, sublane := range sublanes {
			laneLabel := label
			if len(sublanes) > 1 {
				laneLabel = fmt.Sprintf("%s %d", label, i+1)
			}
			lanes = append(lanes, dayOperatorLane{label: laneLabel, blocks: sublane})
		}
	}
	return lanes
}

func assignBlockLanes(blocks []timelineBlock) [][]timelineBlock {
	lanes := make([][]timelineBlock, 0, 2)
	for _, block := range blocks {
		placed := false
		for i := range lanes {
			last := lanes[i][len(lanes[i])-1]
			if !block.start.Before(last.end) {
				lanes[i] = append(lanes[i], block)
				placed = true
				break
			}
		}
		if !placed {
			lanes = append(lanes, []timelineBlock{block})
		}
	}
	return lanes
}

func assignTimelineLanes(entries []model.TimeEntryDetail) [][]timelineBlock {
	lanes := make([][]timelineBlock, 0, 4)
	for _, entry := range entries {
		block := timelineBlock{entry: entry, start: entry.StartedAt, end: timelineBlockEnd(entry)}
		placed := false
		for i := range lanes {
			last := lanes[i][len(lanes[i])-1]
			if !block.start.Before(last.end) {
				lanes[i] = append(lanes[i], block)
				placed = true
				break
			}
		}
		if !placed {
			lanes = append(lanes, []timelineBlock{block})
		}
	}
	return lanes
}

func dayGapsForIndices(entries []model.TimeEntryDetail, indices []int, day time.Time) []dayGap {
	day = dayStart(day)
	dayEnd := gapDayEnd(day)
	if len(indices) == 0 {
		if !dayEnd.After(day) {
			return nil
		}
		return []dayGap{{start: day, end: dayEnd}}
	}
	blocks := make([]timelineBlock, 0, len(indices))
	for _, idx := range indices {
		entry := entries[idx]
		blocks = append(blocks, timelineBlock{entry: entry, start: entry.StartedAt, end: timelineBlockEnd(entry)})
	}
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].start.Before(blocks[j].start) })
	start := day
	mergedStart := blocks[0].start
	mergedEnd := blocks[0].end
	gaps := make([]dayGap, 0, len(blocks)+1)
	if mergedStart.After(start) {
		gaps = append(gaps, dayGap{start: start, end: mergedStart})
	}
	for i := 1; i < len(blocks); i++ {
		if blocks[i].start.After(mergedEnd) {
			gaps = append(gaps, dayGap{start: mergedEnd, end: blocks[i].start})
			mergedStart = blocks[i].start
			mergedEnd = blocks[i].end
			continue
		}
		if blocks[i].end.After(mergedEnd) {
			mergedEnd = blocks[i].end
		}
	}
	if mergedEnd.Before(dayEnd) {
		gaps = append(gaps, dayGap{start: mergedEnd, end: dayEnd})
	}
	filtered := gaps[:0]
	for _, gap := range gaps {
		if gap.end.After(gap.start) {
			filtered = append(filtered, gap)
		}
	}
	return filtered
}

func timelineBlockEnd(entry model.TimeEntryDetail) time.Time {
	if entry.EndedAt != nil {
		if activeAgentEntry(entry, time.Now()) {
			return time.Now().UTC()
		}
		return *entry.EndedAt
	}
	if activeAgentEntry(entry, time.Now()) {
		return time.Now().UTC()
	}
	return entry.StartedAt.Add(time.Minute)
}

func activeAgentEntry(entry model.TimeEntryDetail, now time.Time) bool {
	if entry.Operator == "human" {
		return false
	}
	if entry.EndedAt == nil {
		return true
	}
	end := entry.EndedAt.In(time.UTC)
	now = now.In(time.UTC)
	if end.After(now) {
		return true
	}
	return now.Sub(end) <= 20*time.Minute
}

func renderDayAxis(cols dayTimelineCols, window dayWindow) string {
	runes := make([]rune, cols.Chart)
	for i := range runes {
		runes[i] = '-'
	}
	step := 1
	if window.end.Sub(window.start) > 12*time.Hour {
		step = 2
	}
	for ts := window.start; ts.Before(window.end); ts = ts.Add(time.Duration(step) * time.Hour) {
		col := timelineColumnForTime(ts, cols.Chart, window)
		label := fmt.Sprintf("%02d", ts.Hour())
		for i, r := range label {
			if col+i < len(runes) {
				runes[col+i] = r
			}
		}
	}
	return fmt.Sprintf("%s %s", padRight("time", cols.Lane), string(runes))
}

func renderDayLane(blocks []timelineBlock, m AppModel, width int, window dayWindow, styles tuiStyles) string {
	segments := make([]string, 0, len(blocks)*2+1)
	position := 0
	for _, block := range blocks {
		startCol := timelineColumnForTime(block.start, width, window)
		endCol := timelineColumnForTime(block.end, width, window)
		if endCol <= startCol {
			endCol = minInt(width, startCol+1)
		}
		if startCol > position {
			segments = append(segments, strings.Repeat(" ", startCol-position))
		}
		blockWidth := maxInt(1, endCol-startCol)
		segments = append(segments, renderDayBlock(block, blockWidth, m, styles))
		position = endCol
	}
	if position < width {
		segments = append(segments, strings.Repeat(" ", width-position))
	}
	return strings.Join(segments, "")
}

func renderDayGaps(gaps []dayGap, m AppModel, width int, window dayWindow, styles tuiStyles) string {
	segments := make([]string, 0, len(gaps)*2+1)
	position := 0
	for i, gap := range gaps {
		startCol := timelineColumnForTime(gap.start, width, window)
		endCol := timelineColumnForTime(gap.end, width, window)
		if endCol <= startCol {
			endCol = minInt(width, startCol+1)
		}
		if startCol > position {
			segments = append(segments, strings.Repeat(" ", startCol-position))
		}
		gapWidth := maxInt(1, endCol-startCol)
		segments = append(segments, renderDayGap(gap, i == m.dayGapFocus && m.dayFocusKind == "gap", gapWidth, styles))
		position = endCol
	}
	if position < width {
		segments = append(segments, strings.Repeat(" ", width-position))
	}
	return strings.Join(segments, "")
}

func renderDayBlock(block timelineBlock, width int, m AppModel, styles tuiStyles) string {
	label := timelineBlockLabel(block.entry)
	if width == 1 {
		label = "#"
	} else if width == 2 {
		label = "[]"
	} else {
		label = "[" + padRight(truncateForWidth(label, width-2), width-2) + "]"
	}
	style := styles.baseRow
	if block.entry.Status == model.StatusDraft {
		style = styles.draft
	} else {
		style = styles.confirmed
	}
	if m.dayFocusKind != "gap" && m.entries[m.cursor].ID == block.entry.ID {
		style = styles.activePicker
	}
	if block.entry.ProjectColor != nil && *block.entry.ProjectColor != "" && (m.dayFocusKind == "gap" || m.entries[m.cursor].ID != block.entry.ID) {
		style = style.Foreground(lipgloss.Color(*block.entry.ProjectColor))
	}
	return style.Render(padRight(label, width))
}

func renderDayGap(gap dayGap, focused bool, width int, styles tuiStyles) string {
	label := "gap"
	if width > 6 {
		label = "gap " + formatGapDuration(gap)
	}
	if width == 1 {
		label = "."
	} else if width == 2 {
		label = ".."
	} else {
		label = "{" + padRight(truncateForWidth(label, width-2), width-2) + "}"
	}
	style := styles.muted
	if focused {
		style = styles.activePicker
	}
	return style.Render(padRight(label, width))
}

func formatGapDuration(gap dayGap) string {
	d := gap.end.Sub(gap.start)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

func clock(ts time.Time) string {
	return ts.In(time.Local).Format("15:04")
}

func gapDayEnd(day time.Time) time.Time {
	day = dayStart(day)
	end := day.Add(24 * time.Hour)
	now := time.Now().In(time.Local)
	today := dayStart(now)
	if day.Equal(today) && now.Before(end) {
		return now
	}
	return end
}

func dayStart(ts time.Time) time.Time {
	local := ts.In(time.Local)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
}

func dayKey(ts time.Time) string {
	return ts.In(time.Local).Format("2006-01-02")
}

func timelineBlockLabel(entry model.TimeEntryDetail) string {
	if entry.Description != nil && strings.TrimSpace(*entry.Description) != "" {
		return strings.TrimSpace(*entry.Description)
	}
	parts := []string{entry.Operator}
	if entry.ProjectName != "" {
		parts = append(parts, entry.ProjectName)
	}
	return strings.Join(parts, " ")
}

func timelineColumnForTime(ts time.Time, width int, window dayWindow) int {
	if width <= 1 {
		return 0
	}
	if !window.end.After(window.start) {
		return 0
	}
	if ts.Before(window.start) {
		return 0
	}
	if ts.After(window.end) {
		return width - 1
	}
	offset := ts.Sub(window.start)
	span := window.end.Sub(window.start)
	col := int((offset * time.Duration(width)) / span)
	if col < 0 {
		return 0
	}
	if col >= width {
		return width - 1
	}
	return col
}

func renderStatusBar(m AppModel, width int) string {
	base := renderBaseStatusBar(m, width)
	if m.syncing || m.syncStatusErr != nil {
		return mergeSyncIntoStatusBar(base, m, width)
	}
	return base
}

func renderBaseStatusBar(m AppModel, width int) string {
	if m.mode == modeAssign {
		return truncateForWidth(projectDialogHelp(m), width)
	}
	if m.mode == modeDeleteConfirm {
		return truncateForWidth("delete entry? | y confirm | n/esc cancel", width)
	}
	if m.mode == modeGapEntry {
		return truncateForWidth("gap entry | up/down project | tab focus | enter create | esc cancel", width)
	}
	if m.mode == modeSearch {
		return truncateForWidth("/"+m.searchQuery+" | enter search | esc cancel", width)
	}
	if m.timelineView == timelineViewDay {
		text := fmt.Sprintf("entries %d | day %s | up/down 15m | shift+up/down 1h | j/k items | left/right day | enter create/edit | tab inspector", len(m.entries), m.displayedDay().Format("2006-01-02"))
		return truncateForWidth(text, width)
	}
	drafts := 0
	for _, entry := range m.entries {
		if entry.Status == model.StatusDraft {
			drafts++
		}
	}
	position := "0/0"
	if len(m.entries) > 0 {
		position = strconv.Itoa(m.cursor+1) + "/" + strconv.Itoa(len(m.entries))
	}
	searchHint := ""
	if m.lastSearch != "" {
		searchHint = " | / search n/N next"
	}
	text := fmt.Sprintf("entries %d | drafts %d | pos %s | src %s | r sync | f filter | up/down home/end pgup/pgdn space p assign P projects enter q%s", len(m.entries), drafts, position, m.sourceFilter, searchHint)
	return truncateForWidth(text, width)
}

func mergeSyncIntoStatusBar(base string, m AppModel, width int) string {
	rightWidth := minInt(24, maxInt(14, width/5))
	leftWidth := maxInt(0, width-rightWidth-1)
	left := padRight(truncateForWidth(stripANSI(base), leftWidth), leftWidth)
	right := renderInlineSyncStatus(m, rightWidth)
	if lipgloss.Width(right) < rightWidth {
		right = strings.Repeat(" ", rightWidth-lipgloss.Width(right)) + right
	}
	return left + " " + right
}

func renderInlineSyncStatus(m AppModel, width int) string {
	label := "Syncing"
	if m.syncStatusErr != nil {
		label = "Sync Error"
		return padRight(label+" "+truncateForWidth(m.syncStatusErr.Error(), maxInt(8, width-lipgloss.Width(label)-1)), width)
	}
	spinner := syncSpinnerFrame(m.syncFrame)
	text := spinner + " " + label
	return padRight(text, width)
}

func oneLine(text string) string {
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.ReplaceAll(text, "\n", "")
	return text
}

func syncSpinnerFrame(frame int) string {
	frames := []string{"|", "/", "-", "\\"}
	idx := frame % len(frames)
	return frames[idx]
}

func (m *AppModel) jumpToSearchMatch(start, direction int, includeCurrent bool) {
	if len(m.entries) == 0 || m.lastSearch == "" {
		return
	}
	query := strings.ToLower(strings.TrimSpace(m.lastSearch))
	if query == "" {
		return
	}
	index := start
	if !includeCurrent {
		index += direction
	}
	for checked := 0; checked < len(m.entries); checked++ {
		if index < 0 {
			index = len(m.entries) - 1
		}
		if index >= len(m.entries) {
			index = 0
		}
		if entryMatchesSearch(m.entries[index], query) {
			m.cursor = index
			m.ensureVisible()
			return
		}
		index += direction
	}
}

func entryMatchesSearch(entry model.TimeEntryDetail, query string) bool {
	parts := []string{entry.ProjectName, string(entry.Status), entry.Operator}
	if entry.Description != nil {
		parts = append(parts, *entry.Description)
	}
	if entry.ProjectCode != nil {
		parts = append(parts, *entry.ProjectCode)
	}
	haystack := strings.ToLower(strings.Join(parts, " "))
	return strings.Contains(haystack, query)
}

func (m *AppModel) applySourceFilter() {
	if m.sourceFilter == "" {
		m.sourceFilter = "all"
	}
	if m.sourceFilter == "all" {
		m.entries = append([]model.TimeEntryDetail(nil), m.allEntries...)
	} else {
		filtered := make([]model.TimeEntryDetail, 0, len(m.allEntries))
		for _, entry := range m.allEntries {
			if entry.Operator == m.sourceFilter {
				filtered = append(filtered, entry)
			}
		}
		m.entries = filtered
	}
	if len(m.entries) == 0 {
		m.cursor = 0
		m.offset = 0
		m.selected = map[string]bool{}
		return
	}
	if m.cursor >= len(m.entries) {
		m.cursor = len(m.entries) - 1
	}
	m.pruneSelection()
	m.ensureVisible()
}

func (m *AppModel) cycleSourceFilter(direction int) {
	filters := []string{"all", "opencode", "codex", "claude-code", "human"}
	idx := 0
	for i, filter := range filters {
		if filter == m.sourceFilter {
			idx = i
			break
		}
	}
	idx += direction
	if idx < 0 {
		idx = len(filters) - 1
	}
	if idx >= len(filters) {
		idx = 0
	}
	m.sourceFilter = filters[idx]
	m.applySourceFilter()
}

func (m *AppModel) pruneSelection() {
	if len(m.selected) == 0 {
		return
	}
	visibleIDs := map[string]bool{}
	for _, entry := range m.entries {
		visibleIDs[entry.ID] = true
	}
	for id := range m.selected {
		if !visibleIDs[id] {
			delete(m.selected, id)
		}
	}
}

func stripANSI(text string) string {
	var b strings.Builder
	inEsc := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if inEsc {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				inEsc = false
			}
			continue
		}
		if c == 0x1b {
			inEsc = true
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func renderProjectDialog(m AppModel, styles tuiStyles, background string) string {
	dialogWidth := projectDialogWidth(m.width)
	innerWidth := maxInt(20, dialogWidth-6)

	var content strings.Builder
	content.WriteString(styles.title.Render(projectDialogTitle(m)) + "\n")
	content.WriteString(styles.muted.Render(projectDialogSummary(m)) + "\n\n")
	if m.dialogMode == projectDialogAssign {
		content.WriteString(renderPickerLine("Unassign", 0, m.projectCursor, styles, innerWidth) + "\n")
	}
	if len(m.projects) == 0 {
		content.WriteString(styles.muted.Render(projectDialogEmpty(m)) + "\n")
	} else {
		for i, project := range m.projects {
			content.WriteString(renderPickerLine(projectDialogLabel(m, project), i+1, m.projectCursor, styles, innerWidth) + "\n")
		}
	}
	if m.dialogMode == projectDialogCreate {
		content.WriteString("\n" + styles.activePicker.Render(truncateForWidth("> "+m.projectInput, innerWidth)))
	}
	content.WriteString("\n" + styles.muted.Render(projectDialogHelp(m)))

	dialog := styles.dialogBox.Width(dialogWidth).Render(strings.TrimRight(content.String(), "\n"))
	return lipgloss.Place(timelineWidth(m.width), dialogHeight(m.height, background, dialog), lipgloss.Center, lipgloss.Center, dialog)
}

func renderGapEntryDialog(m AppModel, styles tuiStyles, background string) string {
	dialogWidth := projectDialogWidth(m.width)
	innerWidth := maxInt(20, dialogWidth-6)
	gap := m.selectedCreateRange()

	var content strings.Builder
	content.WriteString(styles.title.Render("Add Manual Entry") + "\n")
	if gap != nil {
		content.WriteString(styles.muted.Render(fmt.Sprintf("%s-%s | %s", clock(gap.start), clock(gap.end), formatGapDuration(*gap))) + "\n\n")
	}
	inputStyle := styles.muted
	if m.gapInputField == "description" {
		inputStyle = styles.activePicker
	}
	content.WriteString(styles.muted.Render("Description") + "\n")
	content.WriteString(inputStyle.Render(truncateForWidth("> "+textWithCaret(m.gapInput, m.caretVisible, m.gapInputField == "description"), innerWidth)))
	content.WriteString("\n\n")
	content.WriteString(styles.muted.Render("Project") + "\n")
	content.WriteString(renderPickerLine("Unassign", 0, m.gapProjectCursor, styles, innerWidth) + "\n")
	if len(m.projects) == 0 {
		content.WriteString(styles.muted.Render("no projects") + "\n")
	} else {
		for i, project := range m.projects {
			content.WriteString(renderPickerLine(projectDialogLabel(AppModel{}, project), i+1, m.gapProjectCursor, styles, innerWidth) + "\n")
		}
	}
	startStyle := styles.muted
	if m.gapInputField == "start" {
		startStyle = styles.activePicker
	}
	endStyle := styles.muted
	if m.gapInputField == "end" {
		endStyle = styles.activePicker
	}
	content.WriteString("\n" + styles.muted.Render("Start") + "\n")
	content.WriteString(startStyle.Render(truncateForWidth("> "+textWithCaret(m.gapStartInput, m.caretVisible, m.gapInputField == "start"), innerWidth)))
	content.WriteString("\n" + styles.muted.Render("End") + "\n")
	content.WriteString(endStyle.Render(truncateForWidth("> "+textWithCaret(m.gapEndInput, m.caretVisible, m.gapInputField == "end"), innerWidth)))
	content.WriteString("\n\n" + styles.muted.Render("tab focus | enter create | esc cancel"))

	dialog := styles.dialogBox.Width(dialogWidth).Render(strings.TrimRight(content.String(), "\n"))
	return lipgloss.Place(timelineWidth(m.width), dialogHeight(m.height, background, dialog), lipgloss.Center, lipgloss.Center, dialog)
}

func renderEntryEditDialog(m AppModel, styles tuiStyles, background string) string {
	dialogWidth := projectDialogWidth(m.width)
	innerWidth := maxInt(20, dialogWidth-6)
	entry := m.entries[m.cursor]

	var content strings.Builder
	content.WriteString(styles.title.Render(entryEditTitle(m)) + "\n")
	content.WriteString(styles.muted.Render(formatRange(entry.StartedAt, entry.EndedAt)) + "\n\n")
	if !m.entryProjectOnly {
		inputStyle := styles.muted
		if m.entryInputField == "description" {
			inputStyle = styles.activePicker
		}
		content.WriteString(styles.muted.Render("Description") + "\n")
		content.WriteString(inputStyle.Render(truncateForWidth("> "+textWithCaret(m.entryInput, m.caretVisible, m.entryInputField == "description"), innerWidth)))
		content.WriteString("\n\n")
	}
	content.WriteString(styles.muted.Render("Project") + "\n")
	content.WriteString(renderPickerLine("Unassign", 0, m.entryProjectCursor, styles, innerWidth) + "\n")
	for i, project := range m.projects {
		content.WriteString(renderPickerLine(projectDialogLabel(AppModel{}, project), i+1, m.entryProjectCursor, styles, innerWidth) + "\n")
	}
	startStyle := styles.muted
	if m.entryInputField == "start" {
		startStyle = styles.activePicker
	}
	endStyle := styles.muted
	if m.entryInputField == "end" {
		endStyle = styles.activePicker
	}
	content.WriteString("\n" + styles.muted.Render("Start") + "\n")
	content.WriteString(startStyle.Render(truncateForWidth("> "+textWithCaret(m.entryStartInput, m.caretVisible, m.entryInputField == "start"), innerWidth)))
	content.WriteString("\n" + styles.muted.Render("End") + "\n")
	content.WriteString(endStyle.Render(truncateForWidth("> "+textWithCaret(m.entryEndInput, m.caretVisible, m.entryInputField == "end"), innerWidth)))
	content.WriteString("\n\n" + styles.muted.Render(entryEditHelp(m)))

	dialog := styles.dialogBox.Width(dialogWidth).Render(strings.TrimRight(content.String(), "\n"))
	return lipgloss.Place(timelineWidth(m.width), dialogHeight(m.height, background, dialog), lipgloss.Center, lipgloss.Center, dialog)
}

func renderDeleteConfirmDialog(m AppModel, styles tuiStyles, background string) string {
	dialogWidth := minInt(50, maxInt(30, m.width/3))
	var entry model.TimeEntryDetail
	for _, e := range m.entries {
		if e.ID == m.confirmDeleteID {
			entry = e
			break
		}
	}
	var content strings.Builder
	content.WriteString(styles.title.Render("Delete Entry") + "\n\n")
	desc := "(no description)"
	if entry.Description != nil && *entry.Description != "" {
		desc = truncateForWidth(*entry.Description, dialogWidth-6)
	}
	content.WriteString(desc + "\n")
	content.WriteString(styles.muted.Render(formatRange(entry.StartedAt, entry.EndedAt)) + "\n\n")
	content.WriteString("Delete this entry? (y/n)")

	dialog := styles.dialogBox.Width(dialogWidth).Render(strings.TrimRight(content.String(), "\n"))
	return lipgloss.Place(timelineWidth(m.width), dialogHeight(m.height, background, dialog), lipgloss.Center, lipgloss.Center, dialog)
}

func projectDialogSummary(m AppModel) string {
	if m.dialogMode == projectDialogManage {
		project := m.selectedProject()
		if project == nil {
			return "manage active projects"
		}
		state := "non-billable"
		if project.BillableDefault {
			state = "billable"
		}
		return truncateForWidth(project.Name+" | default "+state+" | "+projectColorLabel(*project), 48)
	}
	if m.dialogMode == projectDialogCreate {
		return "new project name"
	}
	if len(m.entries) == 0 {
		return "no entry selected"
	}
	entry := m.entries[m.cursor]
	count := len(m.assignmentTargets(entry.ID))
	if count > 1 {
		return fmt.Sprintf("assign %d selected entries", count)
	}
	if entry.Description != nil && strings.TrimSpace(*entry.Description) != "" {
		return truncateForWidth(strings.TrimSpace(*entry.Description), 48)
	}
	return truncateForWidth(entry.ID, 48)
}

func projectDialogLabel(m AppModel, project model.Project) string {
	prefix := ""
	if m.dialogMode == projectDialogManage {
		if project.BillableDefault {
			prefix = "[billable] "
		} else {
			prefix = "[non-billable] "
		}
	}
	if project.Code == nil || *project.Code == "" {
		return prefix + project.Name
	}
	return prefix + project.Name + " (" + *project.Code + ")"
}

func projectDialogTitle(m AppModel) string {
	switch m.dialogMode {
	case projectDialogManage:
		return "Manage Projects"
	case projectDialogCreate:
		return "New Project"
	default:
		return "Assign Project"
	}
}

func projectDialogEmpty(m AppModel) string {
	if m.dialogMode == projectDialogAssign {
		return "no projects"
	}
	return "no active projects"
}

func projectDialogHelp(m AppModel) string {
	switch m.dialogMode {
	case projectDialogManage:
		return "a new | b billable | c next color | C random | x archive | tab assign | esc close"
	case projectDialogCreate:
		return "type name | enter create | esc back"
	default:
		return "enter assign | tab manage | esc cancel"
	}
}

func entryEditTitle(m AppModel) string {
	if m.entryProjectOnly {
		return "Change Project"
	}
	return "Edit Time Entry"
}

func entryEditHelp(m AppModel) string {
	if m.entryProjectOnly {
		return "up/down project | enter save | esc cancel"
	}
	return "up/down project | tab focus | enter save | esc cancel"
}

func projectColorLabel(project model.Project) string {
	if project.Color == nil || *project.Color == "" {
		return "auto"
	}
	return strings.ToLower(*project.Color)
}

func textWithCaret(text string, visible, active bool) string {
	if !active {
		return text
	}
	if visible {
		return text + "█"
	}
	return text + " "
}

func projectDialogWidth(width int) int {
	available := timelineWidth(width)
	if available <= 40 {
		return available
	}
	return minInt(64, maxInt(40, available-8))
}

func dialogHeight(height int, background, dialog string) int {
	if height > 0 {
		return height
	}
	return maxInt(len(strings.Split(background, "\n")), len(strings.Split(dialog, "\n")))
}

func renderPickerLine(label string, index, current int, styles tuiStyles, width int) string {
	cursor := " "
	if index == current {
		cursor = ">"
	}
	line := lipgloss.NewStyle().MaxWidth(width).Render(fmt.Sprintf("%s %s", cursor, label))
	style := styles.projectPicker
	if index == current {
		style = styles.activePicker
	}
	return style.Width(width).MaxWidth(width).Render(line)
}

func formatRange(start time.Time, end *time.Time) string {
	if end == nil {
		return clock(start) + "-..."
	}
	return clock(start) + "-" + clock(*end)
}

func timelineWidth(width int) int {
	if width <= 0 {
		return 80
	}
	return width
}

func truncateForWidth(text string, width int) string {
	if width <= 0 || len(text) <= width {
		return text
	}
	if width <= 3 {
		return text[:width]
	}
	return text[:width-3] + "..."
}

func padRight(text string, width int) string {
	text = truncateForWidth(text, width)
	if len(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len(text))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
