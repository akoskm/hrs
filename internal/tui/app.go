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
)

type mode string

const (
	modeTimeline mode = "timeline"
	modeAssign   mode = "assign"
	modeSearch   mode = "search"
)

type AppModel struct {
	ctx           context.Context
	store         *db.Store
	allEntries    []model.TimeEntryDetail
	entries       []model.TimeEntryDetail
	projects      []model.Project
	selected      map[string]bool
	searchQuery   string
	lastSearch    string
	sourceFilter  string
	width         int
	height        int
	offset        int
	cursor        int
	projectCursor int
	mode          mode
	err           error
	quitting      bool
}

type timelineRow struct {
	Header     string
	Entry      *model.TimeEntryDetail
	EntryIndex int
}

func NewAppModel(ctx context.Context, store *db.Store) (AppModel, error) {
	entries, err := store.ListEntries(ctx)
	if err != nil {
		return AppModel{}, err
	}
	projects, err := store.ListProjects(ctx)
	if err != nil {
		return AppModel{}, err
	}
	sorted := sortEntries(entries)
	model := AppModel{ctx: ctx, store: store, allEntries: sorted, projects: projects, selected: map[string]bool{}, mode: modeTimeline, sourceFilter: "all"}
	model.applySourceFilter()
	return model, nil
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
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "/":
			m.mode = modeSearch
			m.searchQuery = ""
		case "tab", "f":
			if m.mode == modeTimeline {
				m.cycleSourceFilter(1)
			}
		case "shift+tab", "F":
			if m.mode == modeTimeline {
				m.cycleSourceFilter(-1)
			}
		case "up", "k":
			if m.mode == modeAssign {
				if m.projectCursor > 0 {
					m.projectCursor--
				}
			} else if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case "home":
			if m.mode == modeAssign {
				m.projectCursor = 0
			} else {
				m.cursor = 0
				m.ensureVisible()
			}
		case "end":
			if m.mode == modeAssign {
				m.projectCursor = len(m.projects)
			} else {
				if len(m.entries) > 0 {
					m.cursor = len(m.entries) - 1
					m.ensureVisible()
				}
			}
		case "pgdown", "ctrl+f":
			if m.mode == modeAssign {
				step := maxInt(1, m.timelineRows())
				m.projectCursor = minInt(len(m.projects), m.projectCursor+step)
			} else if len(m.entries) > 0 {
				step := maxInt(1, m.timelineRows())
				m.cursor = minInt(len(m.entries)-1, m.cursor+step)
				m.ensureVisible()
			}
		case "pgup", "ctrl+b":
			if m.mode == modeAssign {
				step := maxInt(1, m.timelineRows())
				m.projectCursor = maxInt(0, m.projectCursor-step)
			} else if len(m.entries) > 0 {
				step := maxInt(1, m.timelineRows())
				m.cursor = maxInt(0, m.cursor-step)
				m.ensureVisible()
			}
		case "}":
			if m.mode == modeTimeline && len(m.entries) > 0 {
				m.cursor = m.jumpGroup(1)
				m.ensureVisible()
			}
		case "{":
			if m.mode == modeTimeline && len(m.entries) > 0 {
				m.cursor = m.jumpGroup(-1)
				m.ensureVisible()
			}
		case "down", "j":
			if m.mode == modeAssign {
				if m.projectCursor < len(m.projects) {
					m.projectCursor++
				}
			} else if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case "enter":
			if m.mode == modeTimeline {
				if len(m.entries) > 0 {
					m.mode = modeAssign
				}
				return m, nil
			}
			if len(m.entries) == 0 {
				m.mode = modeTimeline
				return m, nil
			}
			entry := m.entries[m.cursor]
			targetIDs := m.assignmentTargets(entry.ID)
			var err error
			if m.projectCursor == 0 {
				err = m.unassignEntries(targetIDs)
			} else {
				project := m.projects[m.projectCursor-1]
				err = m.assignEntries(targetIDs, project.ID)
			}
			if err != nil {
				m.err = err
				m.mode = modeTimeline
				return m, nil
			}
			entries, err := m.store.ListEntries(m.ctx)
			if err != nil {
				m.err = err
				m.mode = modeTimeline
				return m, nil
			}
			m.allEntries = sortEntries(entries)
			m.applySourceFilter()
			m.selected = map[string]bool{}
			m.mode = modeTimeline
		case "esc":
			m.mode = modeTimeline
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
				m.mode = modeAssign
			}
		case "n":
			if m.mode == modeTimeline && m.lastSearch != "" {
				m.jumpToSearchMatch(m.cursor, 1, false)
			}
		case "N":
			if m.mode == modeTimeline && m.lastSearch != "" {
				m.jumpToSearchMatch(m.cursor, -1, false)
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureVisible()
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
	sections = append(sections, b.String())
	if m.mode == modeAssign {
		var assign strings.Builder
		assign.WriteString(styles.title.Render("Assign Project") + "\n")
		assign.WriteString(renderPickerLine("Unassign", 0, m.projectCursor, styles, m.width) + "\n")
		if len(m.projects) == 0 {
			assign.WriteString(styles.muted.Render("no projects") + "\n")
		} else {
			for i, project := range m.projects {
				code := ""
				if project.Code != nil && *project.Code != "" {
					code = " (" + *project.Code + ")"
				}
				assign.WriteString(renderPickerLine(project.Name+code, i+1, m.projectCursor, styles, m.width) + "\n")
			}
		}
		sections = append(sections, assign.String())
	}
	if m.mode == modeSearch {
		sections = append(sections, styles.title.Render("Search")+"\n"+styles.activePicker.Render("/"+m.searchQuery))
	}
	sections = append(sections, styles.statusBar.Render(renderStatusBar(m, timelineWidth(m.width))))
	return strings.Join(sections, "\n")
}

type timelineColWidths struct {
	Cursor      int
	Time        int
	Description int
	Status      int
	Project     int
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

func (m AppModel) timelineRowsData() []timelineRow {
	rows := make([]timelineRow, 0, len(m.entries)+8)
	lastDate := ""
	for i, entry := range m.entries {
		day := entry.StartedAt.Format("2006-01-02")
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
	currentDay := m.entries[m.cursor].StartedAt.Format("2006-01-02")
	if direction > 0 {
		for i := m.cursor + 1; i < len(m.entries); i++ {
			day := m.entries[i].StartedAt.Format("2006-01-02")
			if day != currentDay {
				return i
			}
		}
		return m.cursor
	}
	for i := m.cursor - 1; i >= 0; i-- {
		day := m.entries[i].StartedAt.Format("2006-01-02")
		if day != currentDay {
			for i > 0 && m.entries[i-1].StartedAt.Format("2006-01-02") == day {
				i--
			}
			return i
		}
	}
	return m.cursor
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
		statusBar:     lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63")).Padding(0, 1).Width(barWidth),
		tableHeader:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")),
		baseRow:       lipgloss.NewStyle(),
		activeRow:     lipgloss.NewStyle().Background(lipgloss.Color("236")),
		selectedRow:   lipgloss.NewStyle().Background(lipgloss.Color("60")),
		activeSelRow:  lipgloss.NewStyle().Background(lipgloss.Color("99")).Bold(true),
		draft:         lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
		confirmed:     lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
		projectPicker: lipgloss.NewStyle(),
		activePicker:  lipgloss.NewStyle().Background(lipgloss.Color("99")).Foreground(lipgloss.Color("230")).Bold(true),
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
		return time.Now().Format("2006-01-02")
	}
	newest := entries[0].StartedAt.Format("2006-01-02")
	oldest := entries[len(entries)-1].StartedAt.Format("2006-01-02")
	if newest == oldest {
		return newest
	}
	return oldest + " to " + newest
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

func renderStatusBar(m AppModel, width int) string {
	if m.mode == modeSearch {
		return truncateForWidth("/"+m.searchQuery+" | enter search | esc cancel", width)
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
	text := fmt.Sprintf("entries %d | drafts %d | pos %s | src %s | f/tab filter | up/down home/end pgup/pgdn space p enter q%s", len(m.entries), drafts, position, m.sourceFilter, searchHint)
	return truncateForWidth(text, width)
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

func renderPickerLine(label string, index, current int, styles tuiStyles, width int) string {
	cursor := " "
	if index == current {
		cursor = ">"
	}
	line := fmt.Sprintf("%s %s", cursor, label)
	style := styles.projectPicker
	if index == current {
		style = styles.activePicker
	}
	return style.Render(truncateForWidth(line, timelineWidth(width)))
}

func formatRange(start time.Time, end *time.Time) string {
	if end == nil {
		return start.Format("15:04") + "-..."
	}
	return start.Format("15:04") + "-" + end.Format("15:04")
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
