package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"
)

type mode string

type projectDialogMode string
type timeOffDialogMode string

type timelineViewMode string

type inspectorTab string
type reportPreset string

const (
	modeTimeline      mode = "timeline"
	modeReport        mode = "report"
	modeAssign        mode = "assign"
	modeTimeOff       mode = "time-off"
	modeEntryEdit     mode = "entry-edit"
	modeGapEntry      mode = "gap-entry"
	modeSearch        mode = "search"
	modeDeleteConfirm mode = "delete-confirm"

	projectDialogAssign projectDialogMode = "assign"
	projectDialogManage projectDialogMode = "manage"
	projectDialogCreate projectDialogMode = "create"

	timeOffDialogManage timeOffDialogMode = "manage"
	timeOffDialogCreate timeOffDialogMode = "create"
	timeOffDialogRecord timeOffDialogMode = "record"

	timelineViewList  timelineViewMode = "list"
	timelineViewDay   timelineViewMode = "day"
	timelineViewMonth timelineViewMode = "month"

	inspectorOverview inspectorTab = "overview"
	inspectorActions  inspectorTab = "actions"

	reportPresetWeek  reportPreset = "week"
	reportPresetMonth reportPreset = "month"
	reportPresetYear  reportPreset = "year"
)

type AppModel struct {
	ctx                  context.Context
	store                *db.Store
	syncFn               func() error
	allEntries           []model.TimeEntryDetail
	entries              []model.TimeEntryDetail
	projects             []model.Project
	timeOffTypes         []model.TimeOffType
	timeOffRecords       []model.TimeOffDayDetail
	selected             map[string]bool
	searchQuery          string
	lastSearch           string
	width                int
	height               int
	offset               int
	cursor               int
	projectCursor        int
	gapProjectCursor     int
	entryProjectCursor   int
	dialogMode           projectDialogMode
	timeOffDialogMode    timeOffDialogMode
	projectInput         string
	timeOffInput         string
	timeOffFromInput     string
	timeOffFromCursor    int
	timeOffToInput       string
	timeOffToCursor      int
	timeOffProjectCursor int
	timeOffTypeCursor    int
	timeOffField         string
	gapInput             string
	gapStartInput        string
	gapEndInput          string
	gapInputField        string
	gapInputCursor       int
	entryInput           string
	entryStartInput      string
	entryEndInput        string
	entryInputField      string
	entryInputCursor     int
	entryProjectOnly     bool
	dayFocusKind         string
	dayGapFocus          int
	daySlotStart         time.Time
	daySlotSpan          time.Duration
	dayDate              time.Time
	dayWindowStart       time.Time
	syncing              bool
	syncSpinner          spinner.Model
	caretVisible         bool
	lastSyncedAt         *time.Time
	syncStatusErr        error
	timelineView         timelineViewMode
	previousTimelineView timelineViewMode
	inspectorTab         inspectorTab
	inspectorViewKey     string
	inspectorViewport    viewport.Model
	slotMarkStart        time.Time
	slotMarkSpan         time.Duration
	confirmDeleteID      string
	reportPreset         reportPreset
	reportProjectCursor  int
	reportResult         db.ReportResult
	activitySlots        []model.ActivitySlot
	styles               tuiStyles
	stylesWidth          int
	mode                 mode
	err                  error
	quitting             bool
}

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
	model := AppModel{ctx: ctx, store: store, syncFn: syncFn, allEntries: sorted, projects: projects, selected: map[string]bool{}, mode: modeTimeline, dialogMode: projectDialogAssign, timelineView: timelineViewList, previousTimelineView: timelineViewList, inspectorTab: inspectorOverview, styles: newStyles(80), stylesWidth: 80, syncing: syncFn != nil, syncSpinner: newSyncSpinner(), inspectorViewport: newInspectorViewport(20, 1)}
	model.entries = model.allEntries
	if err := model.reloadTimeOffRecords(); err != nil {
		return AppModel{}, err
	}
	return model, nil
}

func (m *AppModel) SetDefaultTimelineView(view timelineViewMode) {
	m.timelineView = view
	if view != timelineViewMonth {
		m.previousTimelineView = view
	}
	if view == timelineViewDay {
		m.focusCurrentEntryInDayView()
		if m.daySlotSpan == 0 {
			m.daySlotSpan = 15 * time.Minute
		}
	}
}

func (m *AppModel) loadActivitySlots() {
	day := m.displayedDay()
	slots, err := m.store.ListActivitySlotsForDay(m.ctx, day)
	if err != nil {
		m.err = err
		return
	}
	m.activitySlots = slots
}

func (m *AppModel) InitializeTodayTimelineView() {
	m.timelineView = timelineViewDay
	m.previousTimelineView = timelineViewDay
	today := dayStart(time.Now())
	m.dayDate = today
	m.dayWindowStart = defaultDayWindowStart(today)
	m.daySlotSpan = 15 * time.Minute
	m.daySlotStart = roundDownToStep(time.Now().In(time.Local), 15*time.Minute)
	m.dayFocusKind = "slot"
	m.loadActivitySlots()
}

func (m *AppModel) cycleInspectorTab(step int) {
	tabs := []inspectorTab{inspectorOverview, inspectorActions}
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

func (m AppModel) Init() tea.Cmd {
	if m.syncFn != nil {
		return tea.Batch(runSyncCmd(m.syncFn), m.syncSpinner.Tick)
	}
	return nil
}

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
		if m.mode == modeTimeOff {
			return m, m.handleTimeOffDialogKey(msg)
		}
		if m.mode == modeAssign {
			return m, m.handleProjectDialogKey(msg)
		}
		if m.mode == modeDeleteConfirm {
			return m, m.handleDeleteConfirmKey(msg)
		}
		if m.mode == modeReport {
			return m.handleReportKey(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			if !m.slotMarkStart.IsZero() {
				m.slotMarkStart = time.Time{}
				m.slotMarkSpan = 0
				m.daySlotSpan = 15 * time.Minute
			}
			if len(m.selected) > 0 {
				m.selected = map[string]bool{}
			}
		case "/":
			m.mode = modeSearch
			m.searchQuery = ""
		case "t":
			if m.timelineView == timelineViewDay {
				m.jumpToToday()
			} else if m.timelineView == timelineViewMonth {
				m.dayDate = dayStart(time.Now())
			}
		case "m":
			if m.mode == modeTimeline {
				m.toggleMonthView()
			}
		case "o":
			if len(m.projects) == 0 {
				return m, nil
			}
			if m.timelineView == timelineViewMonth {
				m.openTimeOffRecordDialog()
			} else {
				m.openTimeOffManageDialog()
			}
		case "tab":
			if m.mode == modeTimeline {
				m.cycleInspectorTab(1)
			}
		case "shift+tab":
			if m.mode == modeTimeline {
				m.cycleInspectorTab(-1)
			}
		case "up":
			if m.timelineView == timelineViewMonth {
				m.moveMonthSelection(-7)
			} else if m.timelineView == timelineViewDay {
				if m.dayFocusKind == "entry" && m.cursor >= 0 && m.cursor < len(m.entries) {
					m.slotBeforeEntry(m.entries[m.cursor])
				} else {
					m.moveSlot(-15*time.Minute, 15*time.Minute)
				}
			} else if m.cursor > 0 {
				m.cursor--
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "k":
			if m.timelineView == timelineViewMonth {
				m.moveMonthSelection(-7)
			} else if m.timelineView == timelineViewDay {
				m.jumpDayItem(-1)
			} else if m.cursor > 0 {
				m.cursor--
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "left", "h":
			if m.timelineView == timelineViewMonth {
				m.moveMonthSelection(-1)
			} else if m.timelineView == timelineViewDay {
				m.shiftDisplayedDay(-1)
			}
		case "right", "l":
			if m.timelineView == timelineViewMonth {
				m.moveMonthSelection(1)
			} else if m.timelineView == timelineViewDay {
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
				m.jumpSlotByDuration(-time.Hour)
			}
		case "shift+down":
			if m.timelineView == timelineViewDay {
				m.jumpSlotByDuration(time.Hour)
			}
		case "pgdown", "ctrl+f", "ctrl+d":
			if m.timelineView == timelineViewDay {
				m.scrollInspectorPage(1)
			} else if len(m.entries) > 0 {
				step := max(1, m.timelineRows())
				m.cursor = min(len(m.entries)-1, m.cursor+step)
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "pgup", "ctrl+b", "ctrl+u":
			if m.timelineView == timelineViewDay {
				m.scrollInspectorPage(-1)
			} else if len(m.entries) > 0 {
				step := max(1, m.timelineRows())
				m.cursor = max(0, m.cursor-step)
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
			} else if m.timelineView == timelineViewMonth {
				m.moveMonthSelection(7)
			} else if m.timelineView == timelineViewDay {
				if m.dayFocusKind == "entry" && m.cursor >= 0 && m.cursor < len(m.entries) {
					m.slotAfterEntry(m.entries[m.cursor])
				} else {
					m.moveSlot(15*time.Minute, 15*time.Minute)
				}
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
			} else if m.timelineView == timelineViewMonth {
				m.moveMonthSelection(7)
			} else if m.timelineView == timelineViewDay {
				m.jumpDayItem(1)
			} else if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.focusCurrentEntryInDayView()
				m.ensureVisible()
			}
		case "enter":
			if m.timelineView == timelineViewMonth {
				m.openSelectedMonthDay()
				return m, nil
			}
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
		case "a", "c":
			if m.timelineView == timelineViewDay && (m.dayFocusKind == "gap" || m.dayFocusKind == "slot") {
				m.openGapEntryDialog()
				return m, cursorBlinkCmd()
			}
		case " ", "space":
			if m.mode == modeTimeline && m.timelineView == timelineViewDay && m.dayFocusKind == "slot" {
				if m.slotMarkStart.IsZero() {
					m.slotMarkStart = m.daySlotStart
					m.slotMarkSpan = m.daySlotSpan
				} else {
					m.slotMarkStart = time.Time{}
					m.slotMarkSpan = 0
				}
			} else if m.mode == modeTimeline && len(m.entries) > 0 {
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
				m.jumpToNow()
			} else if m.mode == modeTimeline && m.lastSearch != "" {
				m.jumpToSearchMatch(m.cursor, 1, false)
			}
		case "N":
			if m.mode == modeTimeline && m.lastSearch != "" {
				m.jumpToSearchMatch(m.cursor, -1, false)
			}
		case "s":
			if m.mode == modeTimeline && m.syncFn != nil && !m.syncing {
				m.syncing = true
				m.syncSpinner = newSyncSpinner()
				m.syncStatusErr = nil
				return m, tea.Batch(runSyncCmd(m.syncFn), m.syncSpinner.Tick)
			}
		case "r":
			if m.mode == modeTimeline {
				m.reportPreset = reportPresetWeek
				if err := m.loadReportPreset(m.reportPreset); err != nil {
					m.err = err
					return m, nil
				}
				m.mode = modeReport
				return m, nil
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
		m.styles = newStyles(msg.Width)
		m.stylesWidth = msg.Width
		m.ensureVisible()
	case tea.MouseMsg:
		if m.mode == modeTimeline && m.timelineView == timelineViewDay {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.mouseInInspector(msg.X) {
					m.scrollInspectorLines(-5)
				} else {
					m.scrollWindow(-time.Hour)
				}
			case tea.MouseButtonWheelDown:
				if m.mouseInInspector(msg.X) {
					m.scrollInspectorLines(5)
				} else {
					m.scrollWindow(time.Hour)
				}
			}
		}
	case spinner.TickMsg:
		if !m.syncing {
			return m, nil
		}
		var cmd tea.Cmd
		m.syncSpinner, cmd = m.syncSpinner.Update(msg)
		return m, cmd
	case syncDoneMsg:
		m.syncing = false
		if msg.err != nil {
			m.syncStatusErr = msg.err
			return m, nil
		}
		now := time.Now().UTC()
		m.lastSyncedAt = &now
		m.syncStatusErr = nil
		m.syncSpinner = newSyncSpinner()
		m.restoreStateAfterReload()
		if m.mode == modeReport {
			if err := m.loadReportPreset(m.reportPreset); err != nil {
				m.syncStatusErr = err
				return m, nil
			}
		}
	case cursorBlinkMsg:
		if m.mode != modeEntryEdit && m.mode != modeGapEntry && m.mode != modeTimeOff {
			m.caretVisible = false
			return m, nil
		}
		m.caretVisible = !m.caretVisible
		return m, cursorBlinkCmd()
	}
	m.syncInspectorViewport()
	return m, nil
}

func (m AppModel) handleReportKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.mode = modeTimeline
	case "up", "k":
		m.moveReportProjectCursor(-1)
	case "down", "j":
		m.moveReportProjectCursor(1)
	case "r":
		if err := m.loadReportPreset(m.reportPreset); err != nil {
			m.err = err
		}
	case "w":
		m.reportPreset = reportPresetWeek
		if err := m.loadReportPreset(m.reportPreset); err != nil {
			m.err = err
		}
	case "m":
		m.reportPreset = reportPresetMonth
		if err := m.loadReportPreset(m.reportPreset); err != nil {
			m.err = err
		}
	case "y":
		m.reportPreset = reportPresetYear
		if err := m.loadReportPreset(m.reportPreset); err != nil {
			m.err = err
		}
	}
	return m, nil
}

func (m AppModel) View() string {
	if m.quitting {
		return ""
	}
	styles := m.styles
	if m.stylesWidth == 0 {
		styles = newStyles(m.width)
	}
	var sections []string
	showDayLayout := m.mode == modeTimeline && m.timelineView == timelineViewDay
	if !showDayLayout {
		sections = append(sections, styles.header.Render(renderHeader(m.entries, m.width)))
	}
	if m.err != nil {
		sections = append(sections, styles.error.Render("error: "+m.err.Error()))
	}
	var b strings.Builder
	if m.mode == modeReport {
		b.WriteString(renderReportView(m, styles))
	} else if m.timelineView == timelineViewMonth {
		b.WriteString(renderMonthTimeline(m, styles))
	} else if m.timelineView == timelineViewDay {
		inspectorWidth := max(20, m.width/2)
		timelineWidth := max(40, m.width-inspectorWidth-2)
		b.WriteString(renderDayTimelinePane(m, styles, timelineWidth, dayPaneHeight(m.height)))
	} else if len(m.entries) == 0 {
		b.WriteString(styles.title.Render("Timeline") + "\n")
		b.WriteString(styles.muted.Render("no entries") + "\n")
	} else {
		b.WriteString(styles.title.Render("Timeline") + "\n")
		cols := timelineColumns(m.width)
		header := renderTableHeader(cols, styles)
		b.WriteString(header + "\n")
		b.WriteString(styles.rule.Render(strings.Repeat("─", min(lipgloss.Width(stripANSI(header)), timelineWidth(m.width)))))
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
	if showDayLayout {
		inspectorWidth := dayInspectorWidth(m.width)
		inspector := renderInspectorPane(m, styles, inspectorWidth, dayPaneHeight(m.height))
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, inspector)
	}
	sections = append(sections, body)
	if m.mode == modeSearch {
		sections = append(sections, styles.title.Render("Search")+"\n"+styles.activePicker.Render("/"+m.searchQuery))
	}
	statusWidth := max(0, timelineWidth(m.width)-3)
	statusText := renderStatusBar(m, statusWidth)
	if lipgloss.Width(statusText) < statusWidth {
		statusText += strings.Repeat(" ", statusWidth-lipgloss.Width(statusText))
	}
	statusBar := styles.statusBar.Render(statusText)
	if m.mode == modeReport {
		content := strings.Join(sections, "\n")
		content = padViewToHeight(content, max(0, m.height-lipgloss.Height(statusBar)))
		sections = []string{content, statusBar}
	} else {
		sections = append(sections, statusBar)
	}
	view := strings.Join(sections, "\n")
	if m.mode == modeAssign {
		return renderProjectDialog(m, styles, view)
	}
	if m.mode == modeGapEntry {
		return renderGapEntryDialog(m, styles, view)
	}
	if m.mode == modeTimeOff {
		return renderTimeOffDialog(m, styles, view)
	}
	if m.mode == modeEntryEdit {
		return renderEntryEditDialog(m, styles, view)
	}
	if m.mode == modeDeleteConfirm {
		return renderDeleteConfirmDialog(m, styles, view)
	}
	return view
}

func padViewToHeight(view string, height int) string {
	if height <= 0 {
		return view
	}
	current := lipgloss.Height(view)
	if current >= height {
		return view
	}
	return view + strings.Repeat("\n", height-current)
}

type timelineColWidths struct {
	Cursor      int
	Time        int
	Description int
	Status      int
	Project     int
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
	maxOffset := max(0, len(rows)-visible)
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
}

func (m AppModel) visibleRange(total int) (int, int) {
	visible := m.timelineRows()
	start := min(m.offset, max(0, total))
	end := min(total, start+visible)
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
	available := max(35, width-4)
	cols := timelineColWidths{Cursor: 2, Time: 11, Status: 6, Project: 6, Description: 8}
	extra := available - (cols.Cursor + cols.Time + cols.Status + cols.Project + cols.Description)
	if extra > 0 {
		cols.Project += min(8, extra/3)
		extra -= min(8, extra/3)
		cols.Status += min(3, extra/4)
		extra -= min(3, extra/4)
		cols.Description += extra
	}
	return cols
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

func (m *AppModel) toggleMonthView() {
	if m.timelineView == timelineViewMonth {
		if m.previousTimelineView == "" || m.previousTimelineView == timelineViewMonth {
			m.previousTimelineView = timelineViewList
		}
		m.timelineView = m.previousTimelineView
		return
	}
	if m.timelineView != "" {
		m.previousTimelineView = m.timelineView
	}
	if m.dayDate.IsZero() {
		m.dayDate = m.displayedDay()
	}
	m.timelineView = timelineViewMonth
}

func (m *AppModel) moveMonthSelection(days int) {
	if days == 0 {
		return
	}
	next := m.displayedDay().AddDate(0, 0, days)
	m.dayDate = dayStart(next)
}

func (m *AppModel) openSelectedMonthDay() {
	day := m.displayedDay()
	m.timelineView = timelineViewDay
	m.previousTimelineView = timelineViewDay
	m.dayDate = day
	m.loadActivitySlots()
	m.dayWindowStart = defaultDayWindowStart(day)
	m.syncFocusForDisplayedDay()
	if m.dayFocusKind == "entry" {
		m.ensureEntryVisible()
	}
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
	m.loadActivitySlots()
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
	now := time.Now().In(time.Local)
	m.dayDate = today
	m.setSlotFocus(now, 15*time.Minute)
	m.loadActivitySlots()
	m.centerWindowOn(now)
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
	// snap to row boundaries when inside the visible timeline, fall back to fixed step
	dayEntries := dayEntriesForDate(m.entries, day.Format("2006-01-02"))
	window := dayTimelineWindow(dayEntries, day, m.dayWindowStart)
	rows := dayTimelineRows(window, dayPaneHeight(m.height))
	candidate := time.Time{}
	if len(rows) > 0 && !start.Before(rows[0].start) && start.Before(rows[len(rows)-1].end) {
		if delta > 0 {
			for _, row := range rows {
				if row.start.After(start) {
					candidate = row.start
					break
				}
			}
		} else if delta < 0 {
			for i := len(rows) - 1; i >= 0; i-- {
				if rows[i].start.Before(start) {
					candidate = rows[i].start
					break
				}
			}
		}
	}
	if candidate.IsZero() {
		candidate = roundDownToStep(start.Add(delta), span)
	}
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
	// set span to match the row duration at the candidate position if within visible rows
	for _, row := range rows {
		if row.start.Equal(candidate) {
			span = row.end.Sub(row.start)
			break
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

func (m *AppModel) slotAfterEntry(entry model.TimeEntryDetail) {
	day := m.displayedDay()
	dayEntries := dayEntriesForDate(m.entries, day.Format("2006-01-02"))
	window := dayTimelineWindow(dayEntries, day, m.dayWindowStart)
	rows := dayTimelineRows(window, dayPaneHeight(m.height))
	entryEnd := timelineBlockEnd(entry)
	for _, row := range rows {
		if !row.start.Before(entryEnd) {
			m.daySlotStart = row.start
			m.daySlotSpan = row.end.Sub(row.start)
			m.dayFocusKind = "slot"
			m.ensureSlotVisible()
			return
		}
	}
}

func (m *AppModel) slotBeforeEntry(entry model.TimeEntryDetail) {
	day := m.displayedDay()
	dayEntries := dayEntriesForDate(m.entries, day.Format("2006-01-02"))
	window := dayTimelineWindow(dayEntries, day, m.dayWindowStart)
	rows := dayTimelineRows(window, dayPaneHeight(m.height))
	for i := len(rows) - 1; i >= 0; i-- {
		if !rows[i].end.After(entry.StartedAt) {
			m.daySlotStart = rows[i].start
			m.daySlotSpan = rows[i].end.Sub(rows[i].start)
			m.dayFocusKind = "slot"
			m.ensureSlotVisible()
			return
		}
	}
}

func (m *AppModel) jumpSlotByDuration(delta time.Duration) {
	start := m.daySlotStart
	if start.IsZero() {
		start = roundDownToStep(time.Now().In(time.Local), 15*time.Minute)
	}
	// if not on a full hour, first snap to the closest hour in the direction of travel
	local := start.In(time.Local)
	onHour := local.Minute() == 0 && local.Second() == 0
	var candidate time.Time
	if !onHour {
		snapped := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 0, 0, 0, time.Local)
		if delta > 0 {
			snapped = snapped.Add(time.Hour)
		}
		candidate = snapped
	} else {
		candidate = start.Add(delta)
	}
	day := m.displayedDay()
	dayFloor := dayStart(day)
	dayEnd := dayFloor.Add(24 * time.Hour)
	if candidate.Before(dayFloor) {
		candidate = dayFloor
	}
	if !candidate.Before(dayEnd) {
		candidate = dayEnd.Add(-m.daySlotSpan)
	}
	m.daySlotStart = candidate
	m.dayFocusKind = "slot"
	m.ensureSlotVisible()
}

func (m *AppModel) scrollWindow(delta time.Duration) {
	if m.dayWindowStart.IsZero() {
		m.dayWindowStart = defaultDayWindowStart(m.displayedDay())
	}
	candidate := m.dayWindowStart.Add(delta)
	candidate = clampDayWindowStart(candidate, m.displayedDay())
	m.dayWindowStart = candidate
	// snap cursor into visible window if it fell outside
	windowEnd := m.dayWindowStart.Add(10 * time.Hour)
	if m.daySlotStart.Before(m.dayWindowStart) {
		m.daySlotStart = m.dayWindowStart
	} else if !m.daySlotStart.Before(windowEnd) {
		m.daySlotStart = windowEnd.Add(-m.daySlotSpan)
	}
}

func (m *AppModel) centerWindowOn(ts time.Time) {
	local := ts.In(time.Local)
	centered := alignWindowStart(local.Add(-5 * time.Hour))
	m.dayWindowStart = clampDayWindowStart(centered, m.displayedDay())
}

func (m *AppModel) ensureEntryVisible() {
	if m.cursor < 0 || m.cursor >= len(m.entries) {
		return
	}
	entry := m.entries[m.cursor]
	entryMid := entry.StartedAt.Add(timelineBlockEnd(entry).Sub(entry.StartedAt) / 2)
	m.centerWindowOn(entryMid)
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
	day := m.displayedDay().Format("2006-01-02")
	indices := m.dayEntryIndices(day)
	if len(indices) == 0 {
		return
	}
	if m.dayFocusKind == "slot" || m.dayFocusKind == "gap" {
		if direction > 0 {
			for _, idx := range indices {
				if !m.entries[idx].StartedAt.Before(m.daySlotStart) {
					m.cursor = idx
					m.dayFocusKind = "entry"
					m.ensureEntryVisible()
					return
				}
			}
		} else {
			for i := len(indices) - 1; i >= 0; i-- {
				if !timelineBlockEnd(m.entries[indices[i]]).After(m.daySlotStart) {
					m.cursor = indices[i]
					m.dayFocusKind = "entry"
					m.ensureEntryVisible()
					return
				}
			}
		}
		m.cursor = indices[0]
		m.dayFocusKind = "entry"
		m.ensureEntryVisible()
		return
	}
	current := -1
	for i, idx := range indices {
		if idx == m.cursor {
			current = i
			break
		}
	}
	next := current + direction
	if next >= 0 && next < len(indices) {
		m.cursor = indices[next]
		m.dayFocusKind = "entry"
		m.ensureEntryVisible()
	}
}

func (m *AppModel) jumpToNow() {
	if m.timelineView != timelineViewDay {
		return
	}
	now := time.Now().In(time.Local)
	today := dayStart(now)
	m.dayDate = today
	items := m.dayTimelineItems()
	// Find entry closest to now (skip gaps)
	bestIdx := -1
	bestDist := time.Duration(1<<63 - 1)
	for i, item := range items {
		if item.kind != "entry" {
			continue
		}
		mid := item.start.Add(item.end.Sub(item.start) / 2)
		dist := now.Sub(mid)
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		m.focusDayItem(items[bestIdx])
	} else {
		m.setSlotFocus(now, 15*time.Minute)
	}
	m.loadActivitySlots()
	m.centerWindowOn(now)
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
		m.setSlotFocus(item.start, item.end.Sub(item.start))
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
		start := m.daySlotStart
		end := m.daySlotStart.Add(m.daySlotSpan)
		if !m.slotMarkStart.IsZero() {
			markEnd := m.slotMarkStart.Add(m.slotMarkSpan)
			cursorEnd := m.daySlotStart.Add(m.slotMarkSpan)
			start = minTime(m.slotMarkStart, m.daySlotStart)
			end = maxTime(markEnd, cursorEnd)
		}
		gap := dayGap{start: start, end: end}
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

func clampTextCursor(text string, cursor int) int {
	return max(0, min(cursor, len([]rune(text))))
}

func insertTextAtCursor(text string, cursor int, insert string) (string, int) {
	value := []rune(text)
	pos := clampTextCursor(text, cursor)
	inserted := []rune(insert)
	updated := append(append(value[:pos:pos], inserted...), value[pos:]...)
	return string(updated), pos + len(inserted)
}

func backspaceTextAtCursor(text string, cursor int) (string, int) {
	value := []rune(text)
	pos := clampTextCursor(text, cursor)
	if pos == 0 {
		return text, 0
	}
	updated := append(value[:pos-1:pos-1], value[pos:]...)
	return string(updated), pos - 1
}

func deleteWordBackwardAtCursor(text string, cursor int) (string, int) {
	value := []rune(text)
	pos := clampTextCursor(text, cursor)
	if pos == 0 {
		return text, 0
	}
	start := pos - 1
	for start >= 0 && unicode.IsSpace(value[start]) {
		start--
	}
	for start >= 0 && !unicode.IsSpace(value[start]) {
		start--
	}
	start++
	updated := append(value[:start:start], value[pos:]...)
	return string(updated), start
}

func moveCursorByWord(text string, cursor, direction int) int {
	value := []rune(text)
	pos := clampTextCursor(text, cursor)
	if len(value) == 0 || direction == 0 {
		return pos
	}
	if direction < 0 {
		for pos > 0 && unicode.IsSpace(value[pos-1]) {
			pos--
		}
		for pos > 0 && !unicode.IsSpace(value[pos-1]) {
			pos--
		}
		return pos
	}
	for pos < len(value) && unicode.IsSpace(value[pos]) {
		pos++
	}
	for pos < len(value) && !unicode.IsSpace(value[pos]) {
		pos++
	}
	return pos
}

func (m *AppModel) appendEntryFieldInput(text string) {
	switch m.entryInputField {
	case "description":
		m.entryInput, m.entryInputCursor = insertTextAtCursor(m.entryInput, m.entryInputCursor, text)
	case "start":
		m.entryStartInput = appendClockInput(m.entryStartInput, text)
	case "end":
		m.entryEndInput = appendClockInput(m.entryEndInput, text)
	}
}

func (m *AppModel) backspaceEntryFieldInput() {
	switch m.entryInputField {
	case "description":
		m.entryInput, m.entryInputCursor = backspaceTextAtCursor(m.entryInput, m.entryInputCursor)
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

func (m *AppModel) deleteWordEntryFieldInput() {
	switch m.entryInputField {
	case "description":
		m.entryInput, m.entryInputCursor = deleteWordBackwardAtCursor(m.entryInput, m.entryInputCursor)
	default:
		m.backspaceEntryFieldInput()
	}
}

func (m *AppModel) moveEntryFieldCursorWord(direction int) {
	if m.entryInputField == "description" {
		m.entryInputCursor = moveCursorByWord(m.entryInput, m.entryInputCursor, direction)
	}
}

func (m *AppModel) clearEntryFieldInput() {
	switch m.entryInputField {
	case "description":
		m.entryInput = ""
		m.entryInputCursor = 0
	case "start":
		m.entryStartInput = ""
	case "end":
		m.entryEndInput = ""
	}
}

func (m *AppModel) appendGapFieldInput(text string) {
	switch m.gapInputField {
	case "description":
		m.gapInput, m.gapInputCursor = insertTextAtCursor(m.gapInput, m.gapInputCursor, text)
	case "start":
		m.gapStartInput = appendClockInput(m.gapStartInput, text)
	case "end":
		m.gapEndInput = appendClockInput(m.gapEndInput, text)
	}
}

func appendClockInput(current, text string) string {
	for _, r := range text {
		if len(current) >= 5 {
			break
		}
		pos := len(current)
		switch pos {
		case 0, 1, 3, 4:
			if r < '0' || r > '9' {
				continue
			}
		case 2:
			if r != ':' {
				continue
			}
		}
		current += string(r)
	}
	return current
}

func (m *AppModel) backspaceGapFieldInput() {
	switch m.gapInputField {
	case "description":
		m.gapInput, m.gapInputCursor = backspaceTextAtCursor(m.gapInput, m.gapInputCursor)
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

func (m *AppModel) deleteWordGapFieldInput() {
	switch m.gapInputField {
	case "description":
		m.gapInput, m.gapInputCursor = deleteWordBackwardAtCursor(m.gapInput, m.gapInputCursor)
	default:
		m.backspaceGapFieldInput()
	}
}

func (m *AppModel) moveGapFieldCursorWord(direction int) {
	if m.gapInputField == "description" {
		m.gapInputCursor = moveCursorByWord(m.gapInput, m.gapInputCursor, direction)
	}
}

func (m *AppModel) clearGapFieldInput() {
	switch m.gapInputField {
	case "description":
		m.gapInput = ""
		m.gapInputCursor = 0
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
	m.slotMarkStart = time.Time{}
	m.slotMarkSpan = 0
	m.mode = modeGapEntry
	m.gapInput = ""
	m.gapInputField = "description"
	m.gapInputCursor = 0
	m.gapStartInput = clock(rng.start)
	m.gapEndInput = clock(rng.end)
	m.gapProjectCursor = 0
	m.err = nil
	m.caretVisible = true
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
	m.entryInputCursor = len([]rune(m.entryInput))
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
	m.entryInputCursor = 0
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
	if !m.entryProjectOnly && msg.Type == tea.KeyRunes && !msg.Alt && entryEditFieldIsText(m.entryInputField) {
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
	case "left":
		if !m.entryProjectOnly && m.entryInputField == "description" {
			m.entryInputCursor = max(0, m.entryInputCursor-1)
		}
	case "home":
		if !m.entryProjectOnly && m.entryInputField == "description" {
			m.entryInputCursor = 0
		}
	case "end":
		if !m.entryProjectOnly && m.entryInputField == "description" {
			m.entryInputCursor = len([]rune(m.entryInput))
		}
	case "right":
		if !m.entryProjectOnly && m.entryInputField == "description" {
			m.entryInputCursor = min(len([]rune(m.entryInput)), m.entryInputCursor+1)
		}
	case "alt+left", "alt+b":
		if !m.entryProjectOnly {
			m.moveEntryFieldCursorWord(-1)
		}
	case "alt+right", "alt+f":
		if !m.entryProjectOnly {
			m.moveEntryFieldCursorWord(1)
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
	case "alt+backspace", "ctrl+w":
		if !m.entryProjectOnly {
			m.deleteWordEntryFieldInput()
		}
	case " ", "space":
		if !m.entryProjectOnly && m.entryInputField == "description" {
			m.entryInput, m.entryInputCursor = insertTextAtCursor(m.entryInput, m.entryInputCursor, " ")
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
	m.gapInputCursor = 0
	m.gapProjectCursor = 0
	m.caretVisible = false
}

func (m *AppModel) handleGapEntryKey(msg tea.KeyMsg) tea.Cmd {
	if msg.Type == tea.KeyRunes && !msg.Alt && gapEntryFieldIsText(m.gapInputField) {
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
	case "left":
		if m.gapInputField == "description" {
			m.gapInputCursor = max(0, m.gapInputCursor-1)
		}
	case "home":
		if m.gapInputField == "description" {
			m.gapInputCursor = 0
		}
	case "end":
		if m.gapInputField == "description" {
			m.gapInputCursor = len([]rune(m.gapInput))
		}
	case "right":
		if m.gapInputField == "description" {
			m.gapInputCursor = min(len([]rune(m.gapInput)), m.gapInputCursor+1)
		}
	case "alt+left", "alt+b":
		m.moveGapFieldCursorWord(-1)
	case "alt+right", "alt+f":
		m.moveGapFieldCursorWord(1)
	case "up", "k":
		if m.gapProjectCursor > 0 {
			m.gapProjectCursor--
		}
	case "down", "j":
		if m.gapProjectCursor < len(m.projects) {
			m.gapProjectCursor++
		}
	case "backspace":
		m.backspaceGapFieldInput()
	case "alt+backspace", "ctrl+w":
		m.deleteWordGapFieldInput()
	case " ", "space":
		if m.gapInputField == "description" {
			m.gapInput, m.gapInputCursor = insertTextAtCursor(m.gapInput, m.gapInputCursor, " ")
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

func (m *AppModel) handleTimeOffDialogKey(msg tea.KeyMsg) tea.Cmd {
	if m.timeOffDialogMode == timeOffDialogCreate {
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return tea.Quit
		case "esc":
			m.timeOffDialogMode = timeOffDialogManage
			m.timeOffInput = ""
		case "enter":
			m.createTimeOffType()
		case "backspace":
			if len(m.timeOffInput) > 0 {
				m.timeOffInput = m.timeOffInput[:len(m.timeOffInput)-1]
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.timeOffInput += string(msg.Runes)
			}
		}
		return nil
	}
	if m.timeOffDialogMode == timeOffDialogRecord && timeOffFieldIsDate(m.timeOffField) {
		if msg.Type == tea.KeyRunes && !msg.Alt {
			m.appendTimeOffDateInput(string(msg.Runes))
			return nil
		}
	}

	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return tea.Quit
	case "esc":
		m.closeTimeOffDialog()
	case "tab":
		if m.timeOffDialogMode == timeOffDialogRecord {
			switch m.timeOffField {
			case "project":
				m.timeOffField = "type"
			case "type":
				m.timeOffField = "from"
			case "from":
				m.timeOffField = "to"
			default:
				m.timeOffField = "project"
			}
		} else if m.timeOffField == "project" {
			m.timeOffField = "type"
		} else {
			m.timeOffField = "project"
		}
	case "backspace":
		if m.timeOffDialogMode == timeOffDialogRecord && timeOffFieldIsDate(m.timeOffField) {
			m.backspaceTimeOffDateInput()
		}
	case "up", "k":
		if m.timeOffField == "project" {
			if m.timeOffProjectCursor > 0 {
				m.timeOffProjectCursor--
				m.err = m.reloadTimeOffTypesForSelectedProject()
			}
		} else if m.timeOffTypeCursor > 0 {
			m.timeOffTypeCursor--
		}
	case "down", "j":
		if m.timeOffField == "project" {
			if m.timeOffProjectCursor < len(m.projects)-1 {
				m.timeOffProjectCursor++
				m.err = m.reloadTimeOffTypesForSelectedProject()
			}
		} else {
			maxCursor := len(m.timeOffTypes) - 1
			if m.timeOffDialogMode == timeOffDialogRecord {
				maxCursor = len(m.timeOffTypes)
			}
			if m.timeOffTypeCursor < maxCursor {
				m.timeOffTypeCursor++
			}
		}
	case "a":
		if m.timeOffDialogMode == timeOffDialogManage {
			m.timeOffDialogMode = timeOffDialogCreate
			m.timeOffInput = ""
		}
	case "enter":
		if m.timeOffDialogMode == timeOffDialogRecord {
			m.saveTimeOffRecord()
		}
	}
	return nil
}

func (m *AppModel) createTimeOffType() {
	project := m.selectedTimeOffProject()
	if project == nil {
		m.err = fmt.Errorf("project required")
		return
	}
	item, err := m.store.CreateTimeOffType(m.ctx, project.ID, strings.TrimSpace(m.timeOffInput))
	if err != nil {
		m.err = err
		return
	}
	if err := m.reloadTimeOffTypesForSelectedProject(); err != nil {
		m.err = err
		return
	}
	for i, typeItem := range m.timeOffTypes {
		if typeItem.ID == item.ID {
			m.timeOffTypeCursor = i
			break
		}
	}
	m.timeOffDialogMode = timeOffDialogManage
	m.timeOffInput = ""
}

func (m *AppModel) saveTimeOffRecord() {
	project := m.selectedTimeOffProject()
	if project == nil {
		m.err = fmt.Errorf("project required")
		return
	}
	fromDay, toDay, err := parseDateRangeInputs(m.timeOffFromInput, m.timeOffToInput)
	if err != nil {
		m.err = err
		return
	}
	if m.timeOffTypeCursor == 0 {
		for _, day := range daysInRange(fromDay, toDay) {
			if err := m.store.DeleteTimeOffDay(m.ctx, project.ID, day); err != nil {
				m.err = err
				return
			}
		}
	} else {
		typeItem := m.selectedTimeOffType()
		if typeItem == nil {
			m.err = fmt.Errorf("time off type required")
			return
		}
		for _, day := range daysInRange(fromDay, toDay) {
			if _, err := m.store.UpsertTimeOffDay(m.ctx, project.ID, typeItem.ID, day); err != nil {
				m.err = err
				return
			}
		}
	}
	if err := m.reloadTimeOffRecords(); err != nil {
		m.err = err
		return
	}
	if parsedDay, err := time.ParseInLocation("2006-01-02", fromDay, time.Local); err == nil {
		m.dayDate = dayStart(parsedDay)
	}
	m.closeTimeOffDialog()
}

func (m *AppModel) appendTimeOffDateInput(text string) {
	current := &m.timeOffFromInput
	cursor := &m.timeOffFromCursor
	if m.timeOffField == "to" {
		current = &m.timeOffToInput
		cursor = &m.timeOffToCursor
	}
	for _, r := range text {
		if len(*current) >= 10 {
			break
		}
		pos := len(*current)
		switch pos {
		case 0, 1, 2, 3, 5, 6, 8, 9:
			if r < '0' || r > '9' {
				continue
			}
		case 4, 7:
			if r != '-' {
				continue
			}
		}
		*current += string(r)
	}
	*cursor = len([]rune(*current))
}

func (m *AppModel) backspaceTimeOffDateInput() {
	current := &m.timeOffFromInput
	cursor := &m.timeOffFromCursor
	if m.timeOffField == "to" {
		current = &m.timeOffToInput
		cursor = &m.timeOffToCursor
	}
	if len(*current) == 0 {
		return
	}
	*current = (*current)[:len(*current)-1]
	*cursor = len([]rune(*current))
}

func parseDateInput(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if _, err := time.ParseInLocation("2006-01-02", trimmed, time.Local); err != nil {
		return "", fmt.Errorf("invalid date")
	}
	return trimmed, nil
}

func parseDateRangeInputs(fromValue, toValue string) (string, string, error) {
	fromDay, err := parseDateInput(fromValue)
	if err != nil {
		return "", "", fmt.Errorf("invalid from date")
	}
	toDay, err := parseDateInput(toValue)
	if err != nil {
		return "", "", fmt.Errorf("invalid to date")
	}
	fromTime, _ := time.ParseInLocation("2006-01-02", fromDay, time.Local)
	toTime, _ := time.ParseInLocation("2006-01-02", toDay, time.Local)
	if toTime.Before(fromTime) {
		return "", "", fmt.Errorf("to date must be on or after from date")
	}
	return fromDay, toDay, nil
}

func daysInRange(fromDay, toDay string) []string {
	fromTime, _ := time.ParseInLocation("2006-01-02", fromDay, time.Local)
	toTime, _ := time.ParseInLocation("2006-01-02", toDay, time.Local)
	var days []string
	for day := fromTime; !day.After(toTime); day = day.AddDate(0, 0, 1) {
		days = append(days, day.Format("2006-01-02"))
	}
	return days
}

func timeOffFieldIsDate(field string) bool {
	return field == "from" || field == "to"
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
		step := max(1, m.timelineRows())
		m.projectCursor = min(len(m.projects), m.projectCursor+step)
		if m.projectCursor < m.projectCursorMin() {
			m.projectCursor = m.projectCursorMin()
		}
	case "pgup", "ctrl+b":
		step := max(1, m.timelineRows())
		m.projectCursor = max(m.projectCursorMin(), m.projectCursor-step)
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
	m.projectCursor = min(m.projectCursor, len(m.projects))
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

func (m *AppModel) selectedTimeOffProject() *model.Project {
	if m.timeOffProjectCursor < 0 || m.timeOffProjectCursor >= len(m.projects) {
		return nil
	}
	return &m.projects[m.timeOffProjectCursor]
}

func (m *AppModel) selectedTimeOffType() *model.TimeOffType {
	if m.timeOffTypeCursor <= 0 || m.timeOffTypeCursor > len(m.timeOffTypes) {
		return nil
	}
	return &m.timeOffTypes[m.timeOffTypeCursor-1]
}

func (m *AppModel) openTimeOffManageDialog() {
	m.mode = modeTimeOff
	m.timeOffDialogMode = timeOffDialogManage
	m.timeOffInput = ""
	m.timeOffField = "project"
	m.caretVisible = true
	if m.timeOffProjectCursor >= len(m.projects) {
		m.timeOffProjectCursor = 0
	}
	m.timeOffTypeCursor = 0
	m.err = m.reloadTimeOffTypesForSelectedProject()
}

func (m *AppModel) openTimeOffRecordDialog() {
	m.mode = modeTimeOff
	m.timeOffDialogMode = timeOffDialogRecord
	m.timeOffInput = ""
	m.timeOffFromInput = m.displayedDay().Format("2006-01-02")
	m.timeOffFromCursor = len([]rune(m.timeOffFromInput))
	m.timeOffToInput = m.displayedDay().Format("2006-01-02")
	m.timeOffToCursor = len([]rune(m.timeOffToInput))
	m.timeOffField = "project"
	m.caretVisible = true
	if m.timeOffProjectCursor >= len(m.projects) {
		m.timeOffProjectCursor = 0
	}
	m.timeOffTypeCursor = 0
	if record := m.preferredTimeOffRecordForDay(m.displayedDay()); record != nil {
		for i, project := range m.projects {
			if project.ID == record.ProjectID {
				m.timeOffProjectCursor = i
				break
			}
		}
	}
	m.err = m.reloadTimeOffTypesForSelectedProject()
	if m.err != nil {
		return
	}
	if record := m.preferredTimeOffRecordForDay(m.displayedDay()); record != nil {
		for i, item := range m.timeOffTypes {
			if item.ID == record.TimeOffTypeID {
				m.timeOffTypeCursor = i + 1
				break
			}
		}
		fromDay, toDay := m.timeOffContiguousRange(*record)
		m.timeOffFromInput = fromDay
		m.timeOffFromCursor = len([]rune(fromDay))
		m.timeOffToInput = toDay
		m.timeOffToCursor = len([]rune(toDay))
	}
}

func (m *AppModel) closeTimeOffDialog() {
	m.mode = modeTimeline
	m.timeOffDialogMode = ""
	m.timeOffInput = ""
	m.timeOffFromInput = ""
	m.timeOffFromCursor = 0
	m.timeOffToInput = ""
	m.timeOffToCursor = 0
	m.timeOffField = ""
	m.timeOffTypeCursor = 0
	m.caretVisible = false
}

func (m AppModel) timeOffRecordsForDay(day time.Time) []model.TimeOffDayDetail {
	key := dayStart(day).Format("2006-01-02")
	var records []model.TimeOffDayDetail
	for _, record := range m.timeOffRecords {
		if record.Day == key {
			records = append(records, record)
		}
	}
	return records
}

func (m AppModel) preferredTimeOffRecordForDay(day time.Time) *model.TimeOffDayDetail {
	records := m.timeOffRecordsForDay(day)
	if len(records) == 0 {
		return nil
	}
	if project := m.selectedTimeOffProject(); project != nil {
		for _, record := range records {
			if record.ProjectID == project.ID {
				copy := record
				return &copy
			}
		}
	}
	copy := records[0]
	return &copy
}

func (m AppModel) timeOffContiguousRange(record model.TimeOffDayDetail) (string, string) {
	current, err := time.ParseInLocation("2006-01-02", record.Day, time.Local)
	if err != nil {
		return record.Day, record.Day
	}
	start := current
	for {
		prev := start.AddDate(0, 0, -1).Format("2006-01-02")
		if !m.hasMatchingTimeOffRecord(record.ProjectID, record.TimeOffTypeID, prev) {
			break
		}
		start = start.AddDate(0, 0, -1)
	}
	end := current
	for {
		next := end.AddDate(0, 0, 1).Format("2006-01-02")
		if !m.hasMatchingTimeOffRecord(record.ProjectID, record.TimeOffTypeID, next) {
			break
		}
		end = end.AddDate(0, 0, 1)
	}
	return start.Format("2006-01-02"), end.Format("2006-01-02")
}

func (m AppModel) hasMatchingTimeOffRecord(projectID, typeID, day string) bool {
	for _, record := range m.timeOffRecords {
		if record.ProjectID == projectID && record.TimeOffTypeID == typeID && record.Day == day {
			return true
		}
	}
	return false
}

func (m *AppModel) reloadProjects() error {
	projects, err := m.store.ListProjects(m.ctx)
	if err != nil {
		return err
	}
	m.projects = projects
	return nil
}

func (m *AppModel) reloadTimeOffRecords() error {
	records, err := m.store.ListTimeOffDaysInRange(m.ctx, "0001-01-01", "9999-12-31")
	if err != nil {
		return err
	}
	m.timeOffRecords = records
	return nil
}

func (m *AppModel) reloadTimeOffTypesForSelectedProject() error {
	project := m.selectedTimeOffProject()
	if project == nil {
		m.timeOffTypes = nil
		m.timeOffTypeCursor = 0
		return nil
	}
	if err := m.store.EnsureProjectDefaultTimeOffTypes(m.ctx, project.ID); err != nil {
		return err
	}
	types, err := m.store.ListTimeOffTypesByProject(m.ctx, project.ID)
	if err != nil {
		return err
	}
	m.timeOffTypes = types
	if m.timeOffDialogMode == timeOffDialogManage {
		if len(types) == 0 {
			m.timeOffTypeCursor = 0
		} else {
			m.timeOffTypeCursor = min(m.timeOffTypeCursor, len(types)-1)
		}
	} else {
		m.timeOffTypeCursor = min(m.timeOffTypeCursor, len(types))
	}
	return nil
}

func (m *AppModel) reloadEntries() error {
	entries, err := m.store.ListEntries(m.ctx)
	if err != nil {
		return err
	}
	m.allEntries = sortEntries(entries)
	m.entries = m.allEntries
	return nil
}

func (m *AppModel) restoreStateAfterReload() {
	prevDay := m.displayedDay()
	prevKind := m.dayFocusKind
	prevSlotStart := m.daySlotStart
	prevSlotSpan := m.daySlotSpan
	prevMarkStart := m.slotMarkStart
	prevMarkSpan := m.slotMarkSpan
	prevEntryID := ""
	if len(m.entries) > 0 && m.cursor >= 0 && m.cursor < len(m.entries) {
		prevEntryID = m.entries[m.cursor].ID
	}
	if err := m.reloadEntries(); err != nil {
		m.syncStatusErr = err
		return
	}
	m.dayDate = prevDay
	m.loadActivitySlots()
	if prevKind == "slot" {
		m.dayFocusKind = "slot"
		m.daySlotStart = prevSlotStart
		m.daySlotSpan = prevSlotSpan
		m.slotMarkStart = prevMarkStart
		m.slotMarkSpan = prevMarkSpan
		m.ensureSlotVisible()
		return
	}
	if prevEntryID != "" {
		for i, entry := range m.entries {
			if entry.ID == prevEntryID {
				m.cursor = i
				m.dayFocusKind = "entry"
				m.ensureVisible()
				return
			}
		}
	}
	m.syncFocusForDisplayedDay()
}

func cursorBlinkCmd() tea.Cmd {
	return tea.Tick(530*time.Millisecond, func(time.Time) tea.Msg { return cursorBlinkMsg{} })
}

func newSyncSpinner() spinner.Model {
	return spinner.New(spinner.WithSpinner(spinner.Dot))
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
		header:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")).Width(barWidth),
		title:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")),
		error:         lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
		rule:          lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		dateHeader:    lipgloss.NewStyle().Bold(true),
		muted:         lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		statusBar:     lipgloss.NewStyle().Reverse(true),
		tableHeader:   lipgloss.NewStyle().Bold(true),
		baseRow:       lipgloss.NewStyle(),
		activeRow:     lipgloss.NewStyle().Background(lipgloss.Color("236")),
		selectedRow:   lipgloss.NewStyle().Background(lipgloss.Color("60")),
		activeSelRow:  lipgloss.NewStyle().Reverse(true).Bold(true),
		draft:         lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
		confirmed:     lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true),
		projectPicker: lipgloss.NewStyle(),
		activePicker:  lipgloss.NewStyle().Background(lipgloss.Color("153")).Foreground(lipgloss.Color("235")).Bold(true),
		dialogBox:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("4")).Padding(1, 2),
		inspectorBox:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1),
		inspectorTab:  lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1),
		activeTab:     lipgloss.NewStyle().Reverse(true).Bold(true).Padding(0, 1),
	}
}

func renderHeader(entries []model.TimeEntryDetail, width int) string {
	rangeText := currentRange(entries)
	left := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")).Render("hrs")
	right := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(rangeText)
	spacer := max(1, timelineWidth(width)-lipgloss.Width(left)-lipgloss.Width(right))
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
	dayEntries := dayEntriesForDate(m.entries, selectedDay)
	window := dayTimelineWindow(dayEntries, m.displayedDay(), m.dayWindowStart)
	rows := dayTimelineRows(window, m.height)
	activityColWidth := max(16, timelineWidth(m.width)-8)

	var b strings.Builder
	b.WriteString(styles.dateHeader.Render(renderDateHeader(selectedWeekday+" "+selectedDay, timelineWidth(m.width))) + "\n")
	activityCount := 0
	for _, slot := range m.activitySlots {
		slotLocal := slot.SlotTime.In(time.Local)
		if dayKey(slotLocal) == selectedDay && slot.HasSignal() {
			activityCount++
		}
	}
	totalWorked, breakdown := dayWorkBreakdown(dayEntries)
	subheaderParts := []string{
		fmt.Sprintf("%d entries", len(dayEntries)),
		fmt.Sprintf("%d active slots", activityCount),
		"total " + formatWorkDuration(totalWorked),
	}
	if breakdown != "" {
		subheaderParts = append(subheaderParts, breakdown)
	}
	subheaderParts = append(subheaderParts, fmt.Sprintf("%s-%s", clock(window.start), clock(window.end)))
	subheader := strings.Join(subheaderParts, " | ")
	b.WriteString(styles.muted.Render(truncateForWidth(subheader, timelineWidth(m.width))) + "\n")

	// header
	b.WriteString(padRight("time", 5) + " " + padRight("activity", activityColWidth) + "\n")

	// separator
	b.WriteString(styles.rule.Render(strings.Repeat("-", timelineWidth(m.width))) + "\n")

	// rows
	for _, row := range rows {
		timePart := renderVerticalTimeCell(m, row, styles)
		activityPart := renderActivityCell(m, window.start, row.start, row.end, dayEntries, activityColWidth, styles)
		b.WriteString(timePart + " " + activityPart + "\n")
	}
	if m.dayFocusKind == "slot" && !m.daySlotStart.IsZero() {
		if !m.slotMarkStart.IsZero() {
			rng := m.selectedCreateRange()
			b.WriteString(styles.muted.Render(truncateForWidth(fmt.Sprintf("%s | marking %s-%s | up/down extend | enter create | esc cancel", selectedWeekday, clock(rng.start), clock(rng.end)), timelineWidth(m.width))))
		} else {
			b.WriteString(styles.muted.Render(truncateForWidth(fmt.Sprintf("%s | slot %s-%s | space mark range | enter create or edit overlap", selectedWeekday, clock(m.daySlotStart), clock(m.daySlotStart.Add(m.daySlotSpan))), timelineWidth(m.width))))
		}
		return b.String()
	}
	if gap := m.focusedGap(); gap != nil {
		b.WriteString(styles.muted.Render(truncateForWidth(fmt.Sprintf("%s | focus gap %s-%s | %s | enter add entry", selectedWeekday, clock(gap.start), clock(gap.end), formatGapDuration(*gap)), timelineWidth(m.width))))
		return b.String()
	}
	if len(dayEntries) == 0 {
		b.WriteString(styles.muted.Render(truncateForWidth(fmt.Sprintf("%s | focus empty day | left/right change day | t today", selectedWeekday), timelineWidth(m.width))))
		return b.String()
	}
	selected := m.entries[m.cursor]
	project := "unassigned"
	if selected.ProjectName != "" {
		project = selected.ProjectName
	}
	label := timelineBlockLabel(selected)
	b.WriteString(styles.muted.Render(truncateForWidth(fmt.Sprintf("%s | focus %s | %s | %s | %s", selectedWeekday, formatRange(selected.StartedAt, selected.EndedAt), selected.Operator, truncateForWidth(label, 32), truncateForWidth(project, 20)), timelineWidth(m.width))))
	return b.String()
}

type monthProjectTotal struct {
	name     string
	duration time.Duration
}

type monthTimeOffLabel struct {
	text string
}

type monthDaySummary struct {
	total    time.Duration
	projects []monthProjectTotal
	timeOffs []monthTimeOffLabel
}

func renderMonthTimeline(m AppModel, styles tuiStyles) string {
	selectedDay := m.displayedDay()
	monthStart := time.Date(selectedDay.Year(), selectedDay.Month(), 1, 0, 0, 0, 0, time.Local)
	gridStart := monthGridStart(selectedDay)
	weeks := monthGridWeeks(selectedDay)
	columnWidths := monthColumnWidths(max(7, timelineWidth(m.width)))
	cellHeight := 5
	summaries := monthSummaries(m.entries, m.timeOffRecords)
	weekdays := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

	var b strings.Builder
	b.WriteString(styles.title.Render("Month") + "\n")
	b.WriteString(styles.dateHeader.Render(selectedDay.Format("Jan 2006")) + "\n")

	b.WriteString(styles.tableHeader.Render(strings.Join(weekdays, " ")) + "\n")

	for week := 0; week < weeks; week++ {
		cells := make([]string, 0, 7)
		for dayOffset := 0; dayOffset < 7; dayOffset++ {
			day := gridStart.AddDate(0, 0, week*7+dayOffset)
			cells = append(cells, renderMonthCell(day, monthStart, selectedDay, summaries[dayKey(day)], columnWidths[dayOffset], cellHeight, styles))
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		if week < weeks-1 {
			b.WriteString("\n")
		}
	}
	selectedRecords := m.timeOffRecordsForDay(selectedDay)
	if len(selectedRecords) > 0 {
		parts := make([]string, 0, len(selectedRecords))
		for _, record := range selectedRecords {
			parts = append(parts, record.TimeOffType+" @ "+record.ProjectName)
		}
		b.WriteString("\n" + styles.muted.Render(truncateForWidth("time off | "+strings.Join(parts, " | "), timelineWidth(m.width))))
	}

	return b.String()
}

func renderMonthCell(day, monthStart, selectedDay time.Time, summary monthDaySummary, width, height int, styles tuiStyles) string {
	innerWidth := max(1, width-2)
	lines := []string{strconv.Itoa(day.Day())}
	for _, item := range summary.timeOffs {
		lines = append(lines, item.text)
	}
	if summary.total > 0 {
		lines = append(lines, formatWorkDuration(summary.total))
	}
	available := max(0, height-len(lines))
	if available > 0 && len(summary.projects) > 0 {
		show := min(len(summary.projects), available)
		if len(summary.projects) > available && available > 0 {
			show = max(0, available-1)
		}
		for i := 0; i < show; i++ {
			project := summary.projects[i]
			lines = append(lines, fmt.Sprintf("%s %s", project.name, formatWorkDuration(project.duration)))
		}
		remaining := len(summary.projects) - show
		if remaining > 0 && len(lines) < height {
			lines = append(lines, fmt.Sprintf("+%d more", remaining))
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = padRight(truncateForWidth(lines[i], innerWidth), innerWidth)
	}

	style := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Width(width).Height(height)
	if dayKey(day) == dayKey(selectedDay) {
		style = style.Background(lipgloss.Color("60")).Foreground(lipgloss.Color("255")).Bold(true)
	} else if dayKey(day) == dayKey(time.Now()) {
		style = style.BorderForeground(lipgloss.Color("6"))
	} else if day.Month() != monthStart.Month() {
		style = style.Foreground(lipgloss.Color("8"))
	}
	return style.Render(strings.Join(lines, "\n"))
}

func monthColumnWidths(totalWidth int) []int {
	if totalWidth <= 0 {
		totalWidth = 7
	}
	borderWidth := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).GetHorizontalFrameSize()
	contentWidth := totalWidth - (7 * borderWidth)
	if contentWidth < 7 {
		contentWidth = 7
	}
	base := contentWidth / 7
	remainder := contentWidth % 7
	widths := make([]int, 7)
	for i := range widths {
		widths[i] = base
		if i < remainder {
			widths[i]++
		}
		if widths[i] < 1 {
			widths[i] = 1
		}
	}
	return widths
}

func monthSummaries(entries []model.TimeEntryDetail, timeOffRecords []model.TimeOffDayDetail) map[string]monthDaySummary {
	totals := map[string]map[string]time.Duration{}
	for _, entry := range entries {
		duration := timelineBlockEnd(entry).Sub(entry.StartedAt)
		if duration <= 0 {
			continue
		}
		day := dayKey(entry.StartedAt)
		if totals[day] == nil {
			totals[day] = map[string]time.Duration{}
		}
		project := "unassigned"
		if entry.ProjectName != "" {
			project = entry.ProjectName
		}
		totals[day][project] += duration
	}

	summaries := make(map[string]monthDaySummary, len(totals))
	for day, projects := range totals {
		summary := monthDaySummary{}
		for name, duration := range projects {
			summary.total += duration
			summary.projects = append(summary.projects, monthProjectTotal{name: name, duration: duration})
		}
		sort.Slice(summary.projects, func(i, j int) bool {
			if summary.projects[i].duration == summary.projects[j].duration {
				return summary.projects[i].name < summary.projects[j].name
			}
			return summary.projects[i].duration > summary.projects[j].duration
		})
		summaries[day] = summary
	}
	for _, record := range timeOffRecords {
		summary := summaries[record.Day]
		label := record.TimeOffType + " @ " + record.ProjectName
		summary.timeOffs = append(summary.timeOffs, monthTimeOffLabel{text: label})
		summaries[record.Day] = summary
	}
	return summaries
}

func monthGridStart(day time.Time) time.Time {
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.Local)
	return first.AddDate(0, 0, -monthWeekdayIndex(first))
}

func monthGridWeeks(day time.Time) int {
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.Local)
	totalDays := monthWeekdayIndex(first) + daysInMonth(day)
	weeks := totalDays / 7
	if totalDays%7 != 0 {
		weeks++
	}
	return max(4, weeks)
}

func monthWeekdayIndex(day time.Time) int {
	weekday := day.Weekday()
	if weekday == time.Sunday {
		return 6
	}
	return int(weekday - time.Monday)
}

func daysInMonth(day time.Time) int {
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.Local)
	next := first.AddDate(0, 1, 0)
	return int(next.Sub(first).Hours() / 24)
}

func inspectorHeight(height int) int {
	if height <= 0 {
		return 10
	}
	return min(12, max(8, height/3))
}

func dayPaneHeight(height int) int {
	if height <= 0 {
		return 12
	}
	return max(12, height-1)
}

func renderDayTimelinePane(m AppModel, styles tuiStyles, width int, height int) string {
	innerWidth := max(16, width-6)
	timelineModel := m
	timelineModel.width = innerWidth
	timelineModel.height = max(10, height-1)
	body := renderDayTimeline(timelineModel, styles)
	scrollbar := renderDayScrollbar(timelineModel, styles)
	content := lipgloss.JoinHorizontal(lipgloss.Top, body, scrollbar)
	return styles.inspectorBox.Width(max(20, width-2)).Height(max(1, height-2)).Render(content)
}

func renderInspectorPane(m AppModel, styles tuiStyles, width int, height int) string {
	innerWidth := max(20, width-4)
	tabs := renderInspectorTabs(m, styles)
	bodyHeight := inspectorBodyHeight(height)
	bodyWidth := innerWidth
	scrollbar := ""
	viewport := buildInspectorViewport(m, innerWidth, bodyHeight)
	if inspectorNeedsScrollbar(viewport) {
		bodyWidth = max(1, innerWidth-1)
		viewport = buildInspectorViewport(m, bodyWidth, bodyHeight)
		scrollbar = renderInspectorScrollbar(viewport, bodyHeight, styles)
	}
	body := viewport.View()
	if scrollbar != "" {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, scrollbar)
	}
	content := tabs + "\n" + body
	return styles.inspectorBox.Width(max(20, width-2)).Height(max(1, height-2)).Render(content)
}

func inspectorBodyHeight(height int) int {
	return max(1, height-3)
}

func dayInspectorWidth(width int) int {
	return max(20, width/2)
}

func renderInspectorTabs(m AppModel, styles tuiStyles) string {
	tabs := []struct {
		label string
		id    inspectorTab
	}{
		{label: "Overview", id: inspectorOverview},
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
	return buildInspectorViewport(m, width, height).View()
}

func inspectorLines(m AppModel) []string {
	switch m.inspectorTab {
	case inspectorActions:
		return actionInspectorLines(m)
	default:
		return overviewInspectorLines(m)
	}
}

func (m AppModel) effectiveEntryIndex() int {
	if m.dayFocusKind == "slot" {
		return m.overlappingEntryIndexForSlot()
	}
	return m.cursor
}

func overviewInspectorLines(m AppModel) []string {
	if m.dayFocusKind == "slot" && !m.daySlotStart.IsZero() && m.overlappingEntryIndexForSlot() < 0 {
		if rng := m.selectedCreateRange(); rng != nil {
			if lines := m.activitySummaryLines(rng.start, rng.end, true); len(lines) > 0 {
				return lines
			}
			return []string{
				"Time slot",
				fmt.Sprintf("Range: %s-%s", clock(rng.start), clock(rng.end)),
				fmt.Sprintf("Size: %dm", int(rng.end.Sub(rng.start).Minutes())),
				"Action: Enter creates entry or edits overlap",
			}
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
	idx := m.effectiveEntryIndex()
	if len(m.entries) == 0 || idx < 0 || idx >= len(m.entries) {
		return []string{"No entry selected"}
	}
	entry := m.entries[idx]
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
	if activity := m.activitySummaryLines(entry.StartedAt, timelineBlockEnd(entry), false); len(activity) > 0 {
		lines = append(lines, "")
		lines = append(lines, activity...)
	}
	return lines
}

func (m AppModel) activitySummaryLines(start, end time.Time, includeRange bool) []string {
	slots := m.activitySlotsForRange(start, end)
	if len(slots) == 0 {
		return nil
	}
	lines := make([]string, 0, len(slots)*6)
	if includeRange {
		lines = append(lines, fmt.Sprintf("Activity: %s-%s", clock(start), clock(end)), "")
	}
	lines = append(lines, fmt.Sprintf("Agent activity: %d %s", len(slots), pluralize(len(slots), "slot", "slots")))
	for _, slot := range slots {
		lines = append(lines, fmt.Sprintf("%s (%d msgs)", slot.Operator, slot.MsgCount))
		if slot.GitBranch != "" {
			lines = append(lines, fmt.Sprintf("Branch: %s", slot.GitBranch))
		}
		if slot.Cwd != "" {
			lines = append(lines, fmt.Sprintf("Folder: %s", slot.Cwd))
		}
		if slot.TokenInput > 0 || slot.TokenOutput > 0 {
			lines = append(lines, fmt.Sprintf("Tokens: %s in / %s out", formatTokenCount(slot.TokenInput), formatTokenCount(slot.TokenOutput)))
		}
		if len(slot.UserTexts) > 0 {
			lines = append(lines, "", "Prompts:")
			for i, text := range slot.UserTexts {
				lines = append(lines, fmt.Sprintf("  %d. %s", i+1, text))
			}
		}
	}
	return lines
}

func (m *AppModel) scrollInspectorPage(direction int) {
	if direction == 0 {
		return
	}
	m.syncInspectorViewport()
	if direction > 0 {
		m.inspectorViewport.PageDown()
		return
	}
	m.inspectorViewport.PageUp()
}

func (m *AppModel) scrollInspectorLines(delta int) {
	if delta == 0 {
		return
	}
	m.syncInspectorViewport()
	if delta > 0 {
		m.inspectorViewport.ScrollDown(delta)
		return
	}
	m.inspectorViewport.ScrollUp(-delta)
}

func (m AppModel) mouseInInspector(x int) bool {
	if m.timelineView != timelineViewDay || m.width <= 0 {
		return false
	}
	return x >= max(0, m.width-dayInspectorWidth(m.width))
}

func (m *AppModel) syncInspectorViewport() {
	width := max(20, dayInspectorWidth(m.width)-4)
	height := inspectorBodyHeight(dayPaneHeight(m.height))
	key := m.inspectorContentKey()
	m.inspectorViewport = buildInspectorViewport(*m, width, height)
	m.inspectorViewKey = key
}

func newInspectorViewport(width int, height int) viewport.Model {
	if width < 0 {
		width = 0
	}
	if height < 1 {
		height = 1
	}
	vp := viewport.New(width, height)
	vp.MouseWheelEnabled = false
	return vp
}

func buildInspectorViewport(m AppModel, width int, height int) viewport.Model {
	vp := m.inspectorViewport
	if vp.Width != width || vp.Height != height || vp.Height == 0 {
		next := newInspectorViewport(width, height)
		next.YOffset = vp.YOffset
		vp = next
	}
	if m.inspectorContentKey() != m.inspectorViewKey {
		vp.GotoTop()
	}
	lines := inspectorLines(m)
	for i, line := range lines {
		lines[i] = truncateForWidth(line, width)
	}
	vp.SetContent(strings.Join(lines, "\n"))
	return vp
}

func inspectorNeedsScrollbar(vp viewport.Model) bool {
	return vp.TotalLineCount() > vp.Height
}

func inspectorScrollbar(totalLines, visibleLines, offset int) (thumbStart, thumbEnd int) {
	if totalLines <= 0 || visibleLines <= 0 || totalLines <= visibleLines {
		return 0, 0
	}
	maxOffset := max(1, totalLines-visibleLines)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	thumbSize := min(3, visibleLines)
	if thumbSize > visibleLines {
		thumbSize = visibleLines
	}
	maxThumbStart := max(0, visibleLines-thumbSize)
	ratio := float64(offset) / float64(maxOffset)
	thumbStart = int(ratio * float64(maxThumbStart))
	thumbEnd = thumbStart + thumbSize
	if thumbEnd > visibleLines {
		thumbEnd = visibleLines
	}
	return thumbStart, thumbEnd
}

func renderInspectorScrollbar(vp viewport.Model, height int, styles tuiStyles) string {
	thumbStart, thumbEnd := inspectorScrollbar(vp.TotalLineCount(), vp.Height, vp.YOffset)
	trackStyle := lipgloss.NewStyle().Background(lipgloss.Color("236"))
	thumbStyle := lipgloss.NewStyle().Background(lipgloss.Color("4"))
	return renderScrollbarColumn(height, thumbStart, thumbEnd, trackStyle, thumbStyle)
}

func (m AppModel) inspectorContentKey() string {
	parts := []string{string(m.timelineView), string(m.inspectorTab), m.displayedDay().Format("2006-01-02"), m.dayFocusKind}
	if rng := m.selectedCreateRange(); rng != nil {
		parts = append(parts, rng.start.UTC().Format(time.RFC3339), rng.end.UTC().Format(time.RFC3339))
	}
	if gap := m.focusedGap(); gap != nil {
		parts = append(parts, gap.start.UTC().Format(time.RFC3339), gap.end.UTC().Format(time.RFC3339))
	}
	idx := m.effectiveEntryIndex()
	if idx >= 0 && idx < len(m.entries) {
		parts = append(parts, m.entries[idx].ID)
	}
	return strings.Join(parts, "|")
}

func (m AppModel) activitySlotsForRange(start, end time.Time) []model.ActivitySlot {
	if !end.After(start) {
		return nil
	}
	slots := make([]model.ActivitySlot, 0, len(m.activitySlots))
	for _, slot := range m.activitySlots {
		if !slot.HasSignal() {
			continue
		}
		slotStart := slot.SlotTime.In(time.Local)
		slotEnd := slotStart.Add(15 * time.Minute)
		if rangesOverlap(start, end, slotStart, slotEnd) {
			slots = append(slots, slot)
		}
	}
	return slots
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func actionInspectorLines(m AppModel) []string {
	if m.dayFocusKind == "slot" {
		return []string{
			"Enter/c: create entry or edit overlap",
			"Up/Down: move 15m",
			"Shift+Up/Down: move 1h",
			"j/k: prev/next item",
			"Left/Right: prev/next day",
		}
	}
	if m.dayFocusKind == "gap" {
		return []string{
			"Enter/c: add manual entry",
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
		"S: sync now",
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
	available := max(10, height-6)
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

func dayScrollbar(totalRows int, windowStart time.Time, day time.Time) (thumbStart, thumbEnd int) {
	if totalRows == 0 {
		return 0, 0
	}
	dayFloor := dayStart(day)
	offset := windowStart.Sub(dayFloor)
	if offset < 0 {
		offset = 0
	}
	maxOffset := 24*time.Hour - 10*time.Hour
	if maxOffset <= 0 {
		maxOffset = time.Hour
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	thumbSize := min(3, totalRows)
	maxThumbStart := max(0, totalRows-thumbSize)
	ratio := float64(offset) / float64(maxOffset)
	thumbStart = int(ratio * float64(maxThumbStart))
	thumbEnd = min(totalRows, thumbStart+thumbSize)
	return thumbStart, thumbEnd
}

func renderDayScrollbar(m AppModel, styles tuiStyles) string {
	dayEntries := dayEntriesForDate(m.entries, m.displayedDay().Format("2006-01-02"))
	window := dayTimelineWindow(dayEntries, m.displayedDay(), m.dayWindowStart)
	rows := dayTimelineRows(window, dayPaneHeight(m.height))
	thumbStart, thumbEnd := dayScrollbar(len(rows), window.start, m.displayedDay())

	// 4 header lines (date, subheader, column header, separator) + row lines + 1 footer
	totalLines := 4 + len(rows) + 1
	trackStyle := lipgloss.NewStyle().Background(lipgloss.Color("236"))
	thumbStyle := lipgloss.NewStyle().Background(lipgloss.Color("4"))
	return renderScrollbarColumn(totalLines, 4+thumbStart, 4+thumbEnd, trackStyle, thumbStyle)
}

func renderScrollbarColumn(height, thumbStart, thumbEnd int, trackStyle, thumbStyle lipgloss.Style) string {
	var sb strings.Builder
	for i := 0; i < height; i++ {
		if i >= thumbStart && i < thumbEnd {
			sb.WriteString(renderScrollbarCell(true, thumbStyle))
		} else {
			sb.WriteString(renderScrollbarCell(false, trackStyle))
		}
		if i < height-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func renderScrollbarCell(thumb bool, style lipgloss.Style) string {
	if lipgloss.ColorProfile() == termenv.Ascii {
		if thumb {
			return "█"
		}
		return "│"
	}
	return style.Render(" ")
}

func timeCellIsSlotHighlighted(m AppModel, row dayTimelineRow) bool {
	if m.dayFocusKind != "slot" || m.daySlotStart.IsZero() {
		return false
	}
	slotStart := m.daySlotStart
	slotEnd := m.daySlotStart.Add(m.daySlotSpan)
	if !m.slotMarkStart.IsZero() {
		slotStart = minTime(m.slotMarkStart, m.daySlotStart)
		slotEnd = maxTime(m.slotMarkStart.Add(m.slotMarkSpan), m.daySlotStart.Add(m.slotMarkSpan))
	}
	return rangesOverlap(row.start, row.end, slotStart, slotEnd)
}

func renderVerticalTimeCell(m AppModel, row dayTimelineRow, styles tuiStyles) string {
	label := padRight(row.label, 5)
	if timeCellIsSlotHighlighted(m, row) {
		return styles.activePicker.Width(5).Render(label)
	}
	return label
}

func (m AppModel) activityTextForSlot(slotStart time.Time) string {
	rounded := slotStart.UTC().Truncate(15 * time.Minute)
	for _, slot := range m.activitySlots {
		if slot.SlotTime.Equal(rounded) && slot.HasSignal() {
			if slot.FirstText != "" {
				return slot.FirstText
			}
			return slot.Operator
		}
	}
	return ""
}

func (m AppModel) slotHasActivity(slotStart time.Time) bool {
	rounded := slotStart.UTC().Truncate(15 * time.Minute)
	for _, slot := range m.activitySlots {
		if slot.SlotTime.Equal(rounded) && slot.HasSignal() {
			return true
		}
	}
	return false
}

func renderActivityCell(m AppModel, viewportStart, slotStart, slotEnd time.Time, entries []model.TimeEntryDetail, width int, styles tuiStyles) string {
	// entries take visual precedence over activity markers
	for _, entry := range entries {
		entryEnd := timelineBlockEnd(entry)
		if !rangesOverlap(slotStart, slotEnd, entry.StartedAt, entryEnd) {
			continue
		}
		touchesAbove := false
		touchesBelow := false
		for _, other := range entries {
			if other.ID == entry.ID {
				continue
			}
			otherEnd := timelineBlockEnd(other)
			if otherEnd.Equal(entry.StartedAt) && sharesVisualBoundary(entry, other) {
				touchesAbove = true
			}
			if other.StartedAt.Equal(entryEnd) && sharesVisualBoundary(entry, other) {
				touchesBelow = true
			}
			if touchesAbove && touchesBelow {
				break
			}
		}
		globalIdx := -1
		for gi, e := range m.entries {
			if e.ID == entry.ID {
				globalIdx = gi
				break
			}
		}
		focused := (m.dayFocusKind == "entry" && globalIdx == m.cursor) || (m.dayFocusKind == "slot" && globalIdx == m.overlappingEntryIndexForSlot())
		cellStyle := styles.confirmed
		if entry.Status == model.StatusDraft {
			cellStyle = styles.draft
		}
		if entry.ProjectColor != nil && *entry.ProjectColor != "" && !focused {
			cellStyle = cellStyle.Foreground(lipgloss.Color(*entry.ProjectColor))
		}
		label := timelineBlockLabel(entry)
		return renderVerticalEntryCell(viewportStart, slotStart, slotEnd, entry.StartedAt, entryEnd, touchesAbove, touchesBelow, focused, width, label, cellStyle, styles)
	}
	// activity marker (faint text)
	text := m.activityTextForSlot(slotStart)
	if text != "" {
		return styles.muted.Render(padRight(truncateForWidth(" "+text, width), width))
	}
	return padRight("", width)
}

func sharesProjectBoundary(a, b model.TimeEntryDetail) bool {
	if a.ProjectID == nil || b.ProjectID == nil {
		return a.ProjectID == nil && b.ProjectID == nil
	}
	return *a.ProjectID == *b.ProjectID
}

func sharesVisualBoundary(a, b model.TimeEntryDetail) bool {
	if !sharesProjectBoundary(a, b) {
		return false
	}
	return timelineBlockLabel(a) == timelineBlockLabel(b)
}

func renderVerticalEntryCell(viewportStart, slotStart, slotEnd, itemStart, itemEnd time.Time, touchesAbove, touchesBelow, focused bool, width int, label string, baseStyle lipgloss.Style, styles tuiStyles) string {
	if focused {
		return renderVerticalRangeCell(viewportStart, slotStart, slotEnd, itemStart, itemEnd, touchesAbove, touchesBelow, true, width, label, baseStyle, styles)
	}
	text := outlinedBlockCellWithViewport(slotStart, slotEnd, viewportStart, itemStart, itemEnd, touchesAbove, touchesBelow, width, label)
	if strings.TrimSpace(text) == "" {
		return padRight("", width)
	}
	return baseStyle.Render(text)
}

func renderVerticalRangeCell(viewportStart, slotStart, slotEnd, itemStart, itemEnd time.Time, touchesAbove, touchesBelow, focused bool, width int, label string, baseStyle lipgloss.Style, styles tuiStyles) string {
	text := outlinedBlockCellWithViewport(slotStart, slotEnd, viewportStart, itemStart, itemEnd, touchesAbove, touchesBelow, width, label)
	if strings.TrimSpace(text) == "" {
		return padRight("", width)
	}
	style := baseStyle
	if focused {
		style = styles.activePicker
	}
	return style.Render(padRight(truncateForWidth(text, width), width))
}

func outlinedBlockCell(slotStart, slotEnd, itemStart, itemEnd time.Time, width int, label string) string {
	return outlinedBlockCellWithViewport(slotStart, slotEnd, time.Time{}, itemStart, itemEnd, false, false, width, label)
}

func outlinedBlockCellWithViewport(slotStart, slotEnd, viewportStart, itemStart, itemEnd time.Time, touchesAbove, touchesBelow bool, width int, label string) string {
	if width <= 1 {
		return "│"
	}
	starts := !itemStart.Before(slotStart) && itemStart.Before(slotEnd)
	ends := itemEnd.After(slotStart) && !itemEnd.After(slotEnd)
	slotDuration := slotEnd.Sub(slotStart)
	hasInteriorRow := slotDuration > 0 && itemEnd.Sub(itemStart) > 2*slotDuration
	preferTopBorderLabel := !hasInteriorRow
	preferInteriorLabel := touchesAbove && preferTopBorderLabel
	midpoint := itemStart.Add(itemEnd.Sub(itemStart) / 2)
	containsMid := !midpoint.Before(slotStart) && midpoint.Before(slotEnd)
	anchoredTop := entryAnchoredAtViewportTop(viewportStart, itemStart)
	topClipped := entryClippedAtViewportTop(viewportStart, slotStart, itemStart)
	topAnchorRow := anchoredTop && slotStart.Equal(viewportStart)
	innerWidth := max(0, width-2)
	fill := strings.Repeat("─", innerWidth)
	space := strings.Repeat(" ", innerWidth)
	if starts && ends {
		if lipgloss.Width(label) > innerWidth && lipgloss.Width(label) <= width {
			return compactBlockLabel(label, width)
		}
		return borderLabelRow('┌', '┐', label, width)
	}
	if topAnchorRow && starts {
		if !hasInteriorRow {
			return borderLabelRow('┌', '┐', label, width)
		}
		return "┌" + fill + "┐"
	}
	if topClipped && ends {
		return "└" + padRight(truncateForWidth(label, innerWidth), innerWidth) + "┘"
	}
	if topClipped {
		return "│" + padRight(truncateForWidth(label, innerWidth), innerWidth) + "│"
	}
	if starts && touchesAbove {
		if preferInteriorLabel && !hasInteriorRow {
			return borderLabelRow('├', '┤', label, width)
		}
		return "├" + fill + "┤"
	}
	if starts {
		if preferTopBorderLabel {
			return borderLabelRow('┌', '┐', label, width)
		}
		if containsMid && !anchoredTop {
			return borderLabelRow('┌', '┐', label, width)
		}
		return "┌" + fill + "┐"
	}
	if ends {
		if preferInteriorLabel && !hasInteriorRow {
			return "└" + fill + "┘"
		}
		if touchesBelow {
			return "│" + space + "│"
		}
		if containsMid && !anchoredTop && preferInteriorLabel {
			return borderLabelRow('└', '┘', label, width)
		}
		if containsMid && !anchoredTop && !preferTopBorderLabel {
			return borderLabelRow('┌', '┐', label, width)
		}
		if touchesBelow {
			return "├" + fill + "┤"
		}
		return "└" + fill + "┘"
	}
	if containsMid && (!preferTopBorderLabel || preferInteriorLabel) {
		return "│" + padRight(truncateForWidth(label, innerWidth), innerWidth) + "│"
	}
	return "│" + space + "│"
}

func borderLabelRow(left, right rune, label string, width int) string {
	if width <= 1 {
		return string(left)
	}
	innerWidth := max(0, width-2)
	prefix := ""
	availableWidth := innerWidth
	if innerWidth > 1 {
		prefix = "─"
		availableWidth--
	}
	trimmed := truncateForWidth(label, availableWidth)
	fill := strings.Repeat("─", max(0, innerWidth-lipgloss.Width(prefix)-lipgloss.Width(trimmed)))
	return string(left) + prefix + trimmed + fill + string(right)
}

func entryAnchoredAtViewportTop(viewportStart, itemStart time.Time) bool {
	if viewportStart.IsZero() {
		return false
	}
	return !itemStart.After(viewportStart)
}

func entryClippedAtViewportTop(viewportStart, slotStart, itemStart time.Time) bool {
	if viewportStart.IsZero() {
		return false
	}
	return slotStart.Equal(viewportStart) && itemStart.Before(viewportStart)
}

func centeredBlockLabel(label string, width int) string {
	if width <= 0 {
		return ""
	}
	trimmed := truncateForWidth(label, width)
	if strings.TrimSpace(trimmed) == "" {
		return strings.Repeat(" ", width)
	}
	return padRight(trimmed, width)
}

func compactBlockLabel(label string, width int) string {
	if width <= 0 {
		return ""
	}
	trimmed := truncateForWidth(label, width)
	if strings.TrimSpace(trimmed) == "" {
		return strings.Repeat(" ", width)
	}
	return padRight(trimmed, width)
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

func dayWorkBreakdown(entries []model.TimeEntryDetail) (time.Duration, string) {
	perProject := map[string]time.Duration{}
	total := time.Duration(0)
	for _, entry := range entries {
		duration := timelineBlockEnd(entry).Sub(entry.StartedAt)
		if duration <= 0 {
			continue
		}
		total += duration
		project := "unassigned"
		if entry.ProjectName != "" {
			project = entry.ProjectName
		}
		perProject[project] += duration
	}
	if len(perProject) == 0 {
		return total, ""
	}
	type projectTotal struct {
		name     string
		duration time.Duration
	}
	projects := make([]projectTotal, 0, len(perProject))
	for name, duration := range perProject {
		projects = append(projects, projectTotal{name: name, duration: duration})
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].duration == projects[j].duration {
			return projects[i].name < projects[j].name
		}
		return projects[i].duration > projects[j].duration
	})
	parts := make([]string, 0, len(projects))
	for _, project := range projects {
		parts = append(parts, fmt.Sprintf("%s %s", project.name, formatWorkDuration(project.duration)))
	}
	return total, strings.Join(parts, ", ")
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
	startHour := max(0, start.Hour()-1)
	endHour := min(24, end.Hour()+1)
	if end.Minute() > 0 || end.Second() > 0 || end.Nanosecond() > 0 {
		endHour = min(24, endHour+1)
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

func formatGapDuration(gap dayGap) string {
	return formatWorkDuration(gap.end.Sub(gap.start))
}

func formatWorkDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
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

func formatTokenCount(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return strconv.Itoa(n)
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
	desc := ""
	if entry.Description != nil {
		desc = strings.TrimSpace(*entry.Description)
	}
	branch := ""
	if entry.GitBranch != nil {
		branch = strings.TrimSpace(*entry.GitBranch)
	}
	if entry.Operator != "human" && desc != "" && branch != "" {
		return desc + " [" + branch + "]"
	}
	if desc != "" {
		return desc
	}
	parts := []string{entry.Operator}
	if branch != "" {
		parts = append(parts, "["+branch+"]")
	}
	if entry.ProjectName != "" {
		parts = append(parts, entry.ProjectName)
	}
	return strings.Join(parts, " ")
}

func renderStatusBar(m AppModel, width int) string {
	base := renderBaseStatusBar(m, width)
	if m.syncing || m.syncStatusErr != nil {
		return mergeSyncIntoStatusBar(base, m, width)
	}
	return base
}

func renderBaseStatusBar(m AppModel, width int) string {
	if m.mode == modeTimeOff {
		return truncateForWidth(timeOffDialogHelp(m), width)
	}
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
	if m.mode == modeReport {
		return truncateForWidth(fmt.Sprintf("report %s | w week | m month | y year | r refresh | esc back", m.reportPreset), width)
	}
	if m.timelineView == timelineViewMonth {
		text := fmt.Sprintf("month %s | arrows/hjkl move | enter open day | o time off | m back | t today", m.displayedDay().Format("2006-01-02"))
		return truncateForWidth(text, width)
	}
	if m.timelineView == timelineViewDay {
		text := fmt.Sprintf("entries %d | day %s | up/down 15m | shift+up/down 1h | j/k items | left/right day | wheel/pgup/pgdn inspector | enter create/edit | tab inspector", len(m.entries), m.displayedDay().Format("2006-01-02"))
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
	text := fmt.Sprintf("entries %d | drafts %d | pos %s | s sync | up/down home/end pgup/pgdn space p assign P projects o time-off enter q%s", len(m.entries), drafts, position, searchHint)
	return truncateForWidth(text, width)
}

func mergeSyncIntoStatusBar(base string, m AppModel, width int) string {
	rightWidth := min(24, max(14, width/5))
	leftWidth := max(0, width-rightWidth-1)
	left := padRight(truncateForWidth(stripANSI(base), leftWidth), leftWidth)
	right := renderInlineSyncStatus(m, rightWidth)
	if lipgloss.Width(right) < rightWidth {
		right = strings.Repeat(" ", rightWidth-lipgloss.Width(right)) + right
	}
	return left + " " + right
}

func (m *AppModel) loadReportPreset(preset reportPreset) error {
	start, end := reportRangeForPreset(preset, time.Now().In(time.Local))
	result, err := m.store.RangeReport(m.ctx, start, end)
	if err != nil {
		return err
	}
	m.reportPreset = preset
	m.reportResult = result
	m.clampReportProjectCursor()
	return nil
}

func (m *AppModel) moveReportProjectCursor(delta int) {
	if len(m.reportResult.Projects) == 0 || delta == 0 {
		return
	}
	m.reportProjectCursor += delta
	m.clampReportProjectCursor()
}

func (m *AppModel) clampReportProjectCursor() {
	if len(m.reportResult.Projects) == 0 {
		m.reportProjectCursor = 0
		return
	}
	if m.reportProjectCursor < 0 {
		m.reportProjectCursor = 0
	}
	if m.reportProjectCursor >= len(m.reportResult.Projects) {
		m.reportProjectCursor = len(m.reportResult.Projects) - 1
	}
}

func (m AppModel) selectedReportProject() *db.ReportProjectRow {
	if len(m.reportResult.Projects) == 0 {
		return nil
	}
	idx := m.reportProjectCursor
	if idx < 0 || idx >= len(m.reportResult.Projects) {
		idx = 0
	}
	return &m.reportResult.Projects[idx]
}

func reportRangeForPreset(preset reportPreset, now time.Time) (time.Time, time.Time) {
	switch preset {
	case reportPresetMonth:
		return reportMonthRange(now)
	case reportPresetYear:
		return reportYearRange(now)
	default:
		return reportWeekRange(now)
	}
}

func reportWeekRange(now time.Time) (time.Time, time.Time) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start := day.AddDate(0, 0, -(weekday - 1))
	return start, start.AddDate(0, 0, 7)
}

func reportMonthRange(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 1, 0)
}

func reportYearRange(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, now.Location())
	return start, start.AddDate(1, 0, 0)
}

func renderReportView(m AppModel, styles tuiStyles) string {
	contentWidth := timelineWidth(m.width)
	if contentWidth >= 80 {
		leftWidth := max(28, contentWidth/3)
		rightWidth := max(32, contentWidth-leftWidth-1)
		left := styles.inspectorBox.Width(max(20, leftWidth-2)).Render(renderReportSummaryPane(m, leftWidth-4, styles))
		right := styles.inspectorBox.Width(max(20, rightWidth-2)).Render(renderReportProjectsPane(m, rightWidth-4))
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

	var b strings.Builder
	b.WriteString(styles.title.Render("Report") + "\n")
	b.WriteString(fmt.Sprintf("Range: %s..%s\n\n", m.reportResult.Range.From, m.reportResult.Range.To))
	b.WriteString("Summary\n")
	b.WriteString(renderReportSummaryBody(m))
	b.WriteString("\n\n")
	b.WriteString(renderReportDaysPane(m, contentWidth))
	b.WriteString("\n\n")
	b.WriteString(renderReportProjectsPane(m, contentWidth))
	return strings.TrimRight(b.String(), "\n")
}

func renderReportSummaryPane(m AppModel, width int, styles tuiStyles) string {
	var b strings.Builder
	b.WriteString(styles.title.Render("Summary") + "\n")
	b.WriteString(styles.muted.Render(truncateForWidth(fmt.Sprintf("Range: %s..%s", m.reportResult.Range.From, m.reportResult.Range.To), width)) + "\n\n")
	b.WriteString(renderReportSummaryBody(m))
	b.WriteString("\n\n")
	b.WriteString(renderReportDaysPane(m, width))
	return strings.TrimRight(b.String(), "\n")
}

func renderReportSummaryBody(m AppModel) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Total hours: %.1f\n", float64(m.reportResult.Summary.TotalSecs)/3600))
	b.WriteString(fmt.Sprintf("Billable hours: %.1f\n", float64(m.reportResult.Summary.BillableSecs)/3600))
	b.WriteString(fmt.Sprintf("Non-billable hours: %.1f\n", float64(m.reportResult.Summary.NonBillableSecs)/3600))
	b.WriteString(fmt.Sprintf("Active days: %d\n", m.reportResult.Summary.ActiveDays))
	b.WriteString(fmt.Sprintf("Average daily hours: %.1f\n", float64(m.reportResult.Summary.AverageDailySecs)/3600))
	if earned := reportSummaryEarnedAmount(m.reportResult.Projects); earned != "" {
		b.WriteString("Earned total: " + earned + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderReportProjectsPane(m AppModel, width int) string {
	var b strings.Builder
	b.WriteString("By project\n")
	maxProjectSecs := 0
	for _, project := range m.reportResult.Projects {
		if project.TotalSecs > maxProjectSecs {
			maxProjectSecs = project.TotalSecs
		}
	}
	for i, project := range m.reportResult.Projects {
		prefix := "  "
		if i == m.reportProjectCursor {
			prefix = "> "
		}
		bar := reportProjectBar(project.TotalSecs, maxProjectSecs, 8)
		line := fmt.Sprintf("%s%s %.1fh %s %s", prefix, project.ProjectName, float64(project.TotalSecs)/3600, reportSharePercent(project.TotalSecs, m.reportResult.Summary.TotalSecs), bar)
		b.WriteString(truncateForWidth(line, width) + "\n")
	}
	if selected := m.selectedReportProject(); selected != nil {
		b.WriteString("\nProject detail\n")
		b.WriteString(truncateForWidth(fmt.Sprintf("Selected project: %s", selected.ProjectName), width) + "\n")
		b.WriteString(truncateForWidth(fmt.Sprintf("Billable hours: %.1f", float64(selected.BillableSecs)/3600), width) + "\n")
		b.WriteString(truncateForWidth(fmt.Sprintf("Non-billable hours: %.1f", float64(selected.NonBillableSecs)/3600), width) + "\n")
		if earned := reportEarnedAmount(*selected); earned != "" {
			b.WriteString(truncateForWidth("Earned: "+earned, width) + "\n")
		}
		if selected.Currency != "" {
			b.WriteString(truncateForWidth(fmt.Sprintf("Currency: %s", selected.Currency), width) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func reportEarnedAmount(project db.ReportProjectRow) string {
	if project.HourlyRate <= 0 || project.BillableSecs <= 0 || project.Currency == "" {
		return ""
	}
	amount := float64(project.BillableSecs) / 3600 * float64(project.HourlyRate) / 100
	return fmt.Sprintf("%.2f %s", amount, project.Currency)
}

func reportSummaryEarnedAmount(projects []db.ReportProjectRow) string {
	if len(projects) == 0 {
		return ""
	}
	totals := map[string]float64{}
	order := []string{}
	for _, project := range projects {
		if project.HourlyRate <= 0 || project.BillableSecs <= 0 || project.Currency == "" {
			continue
		}
		if _, ok := totals[project.Currency]; !ok {
			order = append(order, project.Currency)
		}
		totals[project.Currency] += float64(project.BillableSecs) / 3600 * float64(project.HourlyRate) / 100
	}
	if len(order) == 0 {
		return ""
	}
	parts := make([]string, 0, len(order))
	for _, currency := range order {
		parts = append(parts, fmt.Sprintf("%.2f %s", totals[currency], currency))
	}
	return strings.Join(parts, ", ")
}

func renderReportDaysPane(m AppModel, width int) string {
	var b strings.Builder
	b.WriteString("By day\n")
	maxDaySecs := 0
	for _, day := range m.reportResult.Days {
		if day.TotalSecs > maxDaySecs {
			maxDaySecs = day.TotalSecs
		}
	}
	for _, day := range m.reportResult.Days {
		bar := reportProjectBar(day.TotalSecs, maxDaySecs, 8)
		line := fmt.Sprintf("%s %.1fh %s", day.Date, float64(day.TotalSecs)/3600, bar)
		b.WriteString(truncateForWidth(line, width) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func reportProjectBar(totalSecs, maxSecs, width int) string {
	if totalSecs <= 0 || maxSecs <= 0 || width <= 0 {
		return ""
	}
	filled := (totalSecs*width + maxSecs - 1) / maxSecs
	if filled < 1 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled)
}

func reportSharePercent(totalSecs, totalRangeSecs int) string {
	if totalSecs <= 0 || totalRangeSecs <= 0 {
		return "0%"
	}
	percent := int((float64(totalSecs)/float64(totalRangeSecs))*100 + 0.5)
	return fmt.Sprintf("%d%%", percent)
}

func renderInlineSyncStatus(m AppModel, width int) string {
	label := "Syncing"
	if m.syncStatusErr != nil {
		label = "Sync Error"
		return padRight(label+" "+truncateForWidth(m.syncStatusErr.Error(), max(8, width-lipgloss.Width(label)-1)), width)
	}
	text := m.syncSpinner.View() + " " + label
	if lipgloss.Width(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-lipgloss.Width(text))
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
	innerWidth := max(20, dialogWidth-6)

	var content strings.Builder
	content.WriteString(styles.title.Render(projectDialogTitle(m)) + "\n")
	content.WriteString(styles.muted.Render(projectDialogSummary(m)) + "\n")
	if m.dialogMode == projectDialogManage {
		if project := m.selectedProject(); project != nil {
			content.WriteString(styles.muted.Render("Color: ") + projectColorIndicator(*project) + "\n")
		}
	}
	content.WriteString("\n")
	if m.dialogMode == projectDialogAssign {
		content.WriteString(renderPickerLine("Unassign", 0, m.projectCursor, styles, innerWidth) + "\n")
	}
	if len(m.projects) == 0 {
		content.WriteString(styles.muted.Render(projectDialogEmpty(m)) + "\n")
	} else {
		for i, project := range m.projects {
			content.WriteString(renderProjectPickerLine(m, project, i+1, styles, innerWidth) + "\n")
		}
	}
	if m.dialogMode == projectDialogCreate {
		content.WriteString("\n" + styles.activePicker.Render(truncateForWidth("> "+m.projectInput, innerWidth)))
	}
	content.WriteString("\n" + styles.muted.Render(projectDialogHelp(m)))

	dialog := styles.dialogBox.Width(dialogWidth).Render(strings.TrimRight(content.String(), "\n"))
	return lipgloss.Place(timelineWidth(m.width), dialogHeight(m.height, background, dialog), lipgloss.Center, lipgloss.Center, dialog)
}

func renderTimeOffDialog(m AppModel, styles tuiStyles, background string) string {
	dialogWidth := projectDialogWidth(m.width)
	innerWidth := max(20, dialogWidth-6)

	var content strings.Builder
	content.WriteString(styles.title.Render(timeOffDialogTitle(m)) + "\n")
	content.WriteString(styles.muted.Render(timeOffDialogSummary(m)) + "\n\n")
	if m.timeOffDialogMode == timeOffDialogRecord {
		fromStyle := styles.muted
		if m.timeOffField == "from" {
			fromStyle = lipgloss.NewStyle().Bold(true)
		}
		toStyle := styles.muted
		if m.timeOffField == "to" {
			toStyle = lipgloss.NewStyle().Bold(true)
		}
		content.WriteString(styles.muted.Render("From") + "\n")
		content.WriteString(renderDialogTextInputStyled(m.timeOffFromInput, m.timeOffFromCursor, m.caretVisible, m.timeOffField == "from", innerWidth, fromStyle))
		content.WriteString("\n\n")
		content.WriteString(styles.muted.Render("To") + "\n")
		content.WriteString(renderDialogTextInputStyled(m.timeOffToInput, m.timeOffToCursor, m.caretVisible, m.timeOffField == "to", innerWidth, toStyle))
		content.WriteString("\n\n")
	}
	if len(m.projects) > 0 {
		content.WriteString(styles.muted.Render("Project") + "\n")
		for i, project := range m.projects {
			label := project.Name
			if project.Code != nil && *project.Code != "" {
				label += " (" + *project.Code + ")"
			}
			content.WriteString(renderDialogPickerLine(label, i == m.timeOffProjectCursor, m.timeOffField == "project", styles, innerWidth) + "\n")
		}
	}
	if m.timeOffDialogMode == timeOffDialogCreate {
		content.WriteString("\n" + styles.muted.Render("Type name") + "\n")
		content.WriteString(styles.activePicker.Render(truncateForWidth("> "+m.timeOffInput, innerWidth)))
		content.WriteString("\n\n" + styles.muted.Render("type name | enter create | esc back"))
	} else {
		content.WriteString("\n" + styles.muted.Render("Time Off Type") + "\n")
		if m.timeOffDialogMode == timeOffDialogRecord {
			content.WriteString(renderDialogPickerLine("Clear for selected project", 0 == m.timeOffTypeCursor, m.timeOffField == "type", styles, innerWidth) + "\n")
			for i, item := range m.timeOffTypes {
				content.WriteString(renderDialogPickerLine(item.Name, i+1 == m.timeOffTypeCursor, m.timeOffField == "type", styles, innerWidth) + "\n")
			}
		} else if len(m.timeOffTypes) == 0 {
			content.WriteString(styles.muted.Render("no time off types") + "\n")
		} else {
			for i, item := range m.timeOffTypes {
				content.WriteString(renderDialogPickerLine(item.Name, i == m.timeOffTypeCursor, m.timeOffField == "type", styles, innerWidth) + "\n")
			}
		}
		content.WriteString("\n" + styles.muted.Render(timeOffDialogHelp(m)))
	}

	dialog := styles.dialogBox.Width(dialogWidth).Render(strings.TrimRight(content.String(), "\n"))
	return lipgloss.Place(timelineWidth(m.width), dialogHeight(m.height, background, dialog), lipgloss.Center, lipgloss.Center, dialog)
}

func renderGapEntryDialog(m AppModel, styles tuiStyles, background string) string {
	dialogWidth := projectDialogWidth(m.width)
	innerWidth := max(20, dialogWidth-6)
	gap := m.selectedCreateRange()

	var content strings.Builder
	content.WriteString(styles.title.Render("Add Manual Entry") + "\n")
	if gap != nil {
		content.WriteString(styles.muted.Render(fmt.Sprintf("%s-%s | %s", clock(gap.start), clock(gap.end), formatGapDuration(*gap))) + "\n\n")
	}
	inputStyle := styles.muted
	if m.gapInputField == "description" {
		inputStyle = lipgloss.NewStyle().Bold(true)
	}
	content.WriteString(styles.muted.Render("Description") + "\n")
	content.WriteString(renderDialogTextInputStyled(m.gapInput, m.gapInputCursor, m.caretVisible, m.gapInputField == "description", innerWidth, inputStyle))
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
		startStyle = lipgloss.NewStyle().Bold(true)
	}
	endStyle := styles.muted
	if m.gapInputField == "end" {
		endStyle = lipgloss.NewStyle().Bold(true)
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
	innerWidth := max(20, dialogWidth-6)
	entry := m.entries[m.cursor]

	var content strings.Builder
	content.WriteString(styles.title.Render(entryEditTitle(m)) + "\n")
	content.WriteString(styles.muted.Render(formatRange(entry.StartedAt, entry.EndedAt)) + "\n\n")
	if !m.entryProjectOnly {
		inputStyle := styles.muted
		if m.entryInputField == "description" {
			inputStyle = lipgloss.NewStyle().Bold(true)
		}
		content.WriteString(styles.muted.Render("Description") + "\n")
		content.WriteString(renderDialogTextInputStyled(m.entryInput, m.entryInputCursor, m.caretVisible, m.entryInputField == "description", innerWidth, inputStyle))
		content.WriteString("\n\n")
	}
	content.WriteString(styles.muted.Render("Project") + "\n")
	content.WriteString(renderPickerLine("Unassign", 0, m.entryProjectCursor, styles, innerWidth) + "\n")
	for i, project := range m.projects {
		content.WriteString(renderPickerLine(projectDialogLabel(AppModel{}, project), i+1, m.entryProjectCursor, styles, innerWidth) + "\n")
	}
	startStyle := styles.muted
	if m.entryInputField == "start" {
		startStyle = lipgloss.NewStyle().Bold(true)
	}
	endStyle := styles.muted
	if m.entryInputField == "end" {
		endStyle = lipgloss.NewStyle().Bold(true)
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
	dialogWidth := min(50, max(30, m.width/3))
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

func timeOffDialogTitle(m AppModel) string {
	switch m.timeOffDialogMode {
	case timeOffDialogCreate:
		return "New Time Off Type"
	case timeOffDialogRecord:
		return "Record Time Off"
	default:
		return "Manage Time Off"
	}
}

func timeOffDialogSummary(m AppModel) string {
	project := m.selectedTimeOffProject()
	projectLabel := "no project selected"
	if project != nil {
		projectLabel = project.Name
	}
	if m.timeOffDialogMode == timeOffDialogRecord {
		return fmt.Sprintf("%s to %s | %s", m.timeOffFromInput, m.timeOffToInput, projectLabel)
	}
	if m.timeOffDialogMode == timeOffDialogCreate {
		return projectLabel
	}
	return projectLabel + " | categories"
}

func timeOffDialogHelp(m AppModel) string {
	switch m.timeOffDialogMode {
	case timeOffDialogCreate:
		return "type name | enter create | esc back"
	case timeOffDialogRecord:
		return "tab cycle project/type/from/to | up/down move | enter save | esc cancel"
	default:
		return "tab cycle fields | up/down move | a new | esc close"
	}
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
		return truncateForWidth(project.Name+" | default "+state, 48)
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

func projectColorIndicator(project model.Project) string {
	label := projectColorLabel(project)
	if project.Color == nil || *project.Color == "" {
		return label
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(*project.Color)).Render(label)
}

func textWithCaret(text string, visible, active bool) string {
	return textWithCaretAt(text, len([]rune(text)), visible, active)
}

func renderDialogTextInput(text string, cursor int, visible, active bool, width int) string {
	return renderDialogTextInputStyled(text, cursor, visible, active, width, lipgloss.NewStyle())
}

func renderDialogTextInputStyled(text string, cursor int, visible, active bool, width int, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	prefix := []rune("> ")
	available := max(0, width-len(prefix))
	reserveEndCaret := active && cursor >= len([]rune(text))
	segment, relCursor := dialogTextViewportSegment(text, cursor, available, reserveEndCaret)
	if !active || !visible {
		if reserveEndCaret && available > 0 {
			return style.Render(string(prefix) + padRight(segment, available))
		}
		return style.Render(string(prefix) + segment)
	}
	caretStyle := style.Copy().Reverse(true)
	value := []rune(segment)
	if len(value) == 0 {
		return style.Render(string(prefix)) + caretStyle.Render("▏")
	}
	if relCursor >= len(value) {
		return style.Render(string(prefix)+segment) + caretStyle.Render("▏")
	}
	return style.Render(string(prefix)+string(value[:relCursor])) + caretStyle.Render(string(value[relCursor])) + style.Render(string(value[relCursor+1:]))
}

func dialogTextViewport(text string, cursor int, visible, active bool, width int) string {
	if width <= 0 {
		return ""
	}
	segment, relCursor := dialogTextViewportSegment(text, cursor, width, false)
	if !active || !visible {
		return segment
	}
	return textWithCaretAt(segment, relCursor, true, true)
}

func dialogTextViewportSegment(text string, cursor int, width int, reserveEndCaret bool) (string, int) {
	if width <= 0 {
		return "", 0
	}
	value := []rune(text)
	pos := clampTextCursor(text, cursor)
	available := width
	if reserveEndCaret && available > 0 {
		available--
	}
	if len(value) <= available {
		return text, pos
	}
	start := 0
	if pos >= available {
		start = pos - available + 1
	}
	if start+available > len(value) {
		start = max(0, len(value)-available)
	}
	end := min(len(value), start+available)
	return string(value[start:end]), pos - start
}

func textWithCaretAt(text string, cursor int, visible, active bool) string {
	if !active {
		return text
	}
	value := []rune(text)
	pos := clampTextCursor(text, cursor)
	if !visible {
		return text
	}
	caretStyle := lipgloss.NewStyle().Reverse(true).Bold(true)
	if pos >= len(value) {
		if len(value) == 0 {
			return "▏"
		}
		return string(value[:len(value)-1]) + caretStyle.Render(string(value[len(value)-1]))
	}
	return string(value[:pos]) + caretStyle.Render(string(value[pos])) + string(value[pos+1:])
}

func projectDialogWidth(width int) int {
	available := timelineWidth(width)
	if available <= 40 {
		return available
	}
	return min(64, max(40, available-8))
}

func dialogHeight(height int, background, dialog string) int {
	if height > 0 {
		return height
	}
	return max(len(strings.Split(background, "\n")), len(strings.Split(dialog, "\n")))
}

func renderPickerLine(label string, index, current int, styles tuiStyles, width int) string {
	cursor := " "
	if index == current {
		cursor = ">"
	}
	line := lipgloss.NewStyle().MaxWidth(width).Render(cursor + " " + label)
	style := styles.projectPicker
	if index == current {
		style = styles.activePicker
	}
	return style.Width(width).MaxWidth(width).Render(line)
}

func renderDialogPickerLine(label string, selected bool, active bool, styles tuiStyles, width int) string {
	cursor := " "
	if selected {
		cursor = ">"
	}
	line := lipgloss.NewStyle().MaxWidth(width).Render(cursor + " " + label)
	style := styles.projectPicker
	if active && selected {
		style = styles.activePicker
	}
	return style.Width(width).MaxWidth(width).Render(line)
}

func renderProjectPickerLine(m AppModel, project model.Project, index int, styles tuiStyles, width int) string {
	label := projectDialogLabel(m, project)
	if m.dialogMode == projectDialogManage {
		return renderPickerLine(label, index, m.projectCursor, styles, width)
	}
	return renderPickerLine(label, index, m.projectCursor, styles, width)
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
	if width <= 0 || lipgloss.Width(text) <= width {
		return text
	}
	if width <= 3 {
		return clipToDisplayWidth(text, width)
	}
	return clipToDisplayWidth(text, width-3) + "..."
}

func clipToDisplayWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	current := 0
	for _, r := range text {
		rw := lipgloss.Width(string(r))
		if current+rw > width {
			break
		}
		b.WriteRune(r)
		current += rw
	}
	return b.String()
}

func padRight(text string, width int) string {
	text = truncateForWidth(text, width)
	if len(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len(text))
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
