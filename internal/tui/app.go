package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"
)

type mode string

const (
	modeTimeline mode = "timeline"
	modeAssign   mode = "assign"
)

type AppModel struct {
	ctx           context.Context
	store         *db.Store
	entries       []model.TimeEntryDetail
	projects      []model.Project
	selected      map[string]bool
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
	return AppModel{ctx: ctx, store: store, entries: sortEntries(entries), projects: projects, selected: map[string]bool{}, mode: modeTimeline}, nil
}

func (m AppModel) Init() tea.Cmd { return nil }

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
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
				if len(m.projects) > 0 {
					m.projectCursor = len(m.projects) - 1
				}
			} else {
				if len(m.entries) > 0 {
					m.cursor = len(m.entries) - 1
					m.ensureVisible()
				}
			}
		case "pgdown", "ctrl+f":
			if m.mode == modeAssign {
				step := maxInt(1, m.timelineRows())
				m.projectCursor = minInt(len(m.projects)-1, m.projectCursor+step)
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
		case "down", "j":
			if m.mode == modeAssign {
				if m.projectCursor < len(m.projects)-1 {
					m.projectCursor++
				}
			} else if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case "enter":
			if m.mode == modeTimeline {
				if len(m.entries) > 0 && len(m.projects) > 0 {
					m.mode = modeAssign
				}
				return m, nil
			}
			if len(m.entries) == 0 || len(m.projects) == 0 {
				m.mode = modeTimeline
				return m, nil
			}
			entry := m.entries[m.cursor]
			project := m.projects[m.projectCursor]
			targetIDs := m.assignmentTargets(entry.ID)
			if err := m.assignEntries(targetIDs, project.ID); err != nil {
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
			m.entries = sortEntries(entries)
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
	var b strings.Builder
	b.WriteString("hrs\n\n")
	if m.err != nil {
		b.WriteString("error: " + m.err.Error() + "\n\n")
	}
	b.WriteString("Timeline\n")
	if len(m.entries) == 0 {
		b.WriteString("no entries\n")
	} else {
		cols := timelineColumns(m.width)
		header := fmt.Sprintf("%s %s %s %s %s", padRight(" ", cols.Cursor), padRight("Time", cols.Time), padRight("Description", cols.Description), padRight("Status", cols.Status), padRight("Project", cols.Project))
		b.WriteString(header + "\n")
		b.WriteString(strings.Repeat("-", minInt(len(header), timelineWidth(m.width))) + "\n")
		rows := m.timelineRowsData()
		start, end := m.visibleRange(len(rows))
		for i := start; i < end; i++ {
			row := rows[i]
			if row.Header != "" {
				b.WriteString(truncateForWidth(row.Header, timelineWidth(m.width)) + "\n")
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
			line := fmt.Sprintf("%s %s %s %s %s",
				padRight(cursor, cols.Cursor),
				padRight(formatRange(entry.StartedAt, entry.EndedAt), cols.Time),
				padRight(truncateForWidth(desc, cols.Description), cols.Description),
				padRight(truncateForWidth(string(entry.Status), cols.Status), cols.Status),
				padRight(truncateForWidth(project, cols.Project), cols.Project),
			)
			b.WriteString(truncateForWidth(line, timelineWidth(m.width)) + "\n")
		}
		if len(rows) > end {
			remaining := len(rows) - end
			b.WriteString(truncateForWidth("... "+strconv.Itoa(remaining)+" more", timelineWidth(m.width)) + "\n")
		}
	}
	if m.mode == modeAssign {
		b.WriteString("\nAssign Project\n")
		if len(m.projects) == 0 {
			b.WriteString("no projects\n")
		} else {
			for i, project := range m.projects {
				cursor := " "
				if i == m.projectCursor {
					cursor = ">"
				}
				code := ""
				if project.Code != nil && *project.Code != "" {
					code = " (" + *project.Code + ")"
				}
				b.WriteString(fmt.Sprintf("%s %s%s\n", cursor, project.Name, code))
			}
		}
		b.WriteString("\n" + truncateForWidth("enter confirm, esc cancel", timelineWidth(m.width)) + "\n")
	} else {
		b.WriteString("\n" + truncateForWidth("up/down move, space select, p bulk assign, enter assign, q quit", timelineWidth(m.width)) + "\n")
	}
	return b.String()
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
			rows = append(rows, timelineRow{Header: "-- " + day + " --"})
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

func sortEntries(entries []model.TimeEntryDetail) []model.TimeEntryDetail {
	sorted := append([]model.TimeEntryDetail(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].StartedAt.After(sorted[j].StartedAt)
	})
	return sorted
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
