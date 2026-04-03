package tui

import (
	"context"
	"fmt"
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
	width         int
	height        int
	offset        int
	cursor        int
	projectCursor int
	mode          mode
	err           error
	quitting      bool
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
	return AppModel{ctx: ctx, store: store, entries: entries, projects: projects, mode: modeTimeline}, nil
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
			if err := m.store.AssignEntryToProject(m.ctx, entry.ID, project.ID); err != nil {
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
			m.entries = entries
			m.mode = modeTimeline
		case "esc":
			m.mode = modeTimeline
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
		start, end := m.visibleRange()
		for i := start; i < end; i++ {
			entry := m.entries[i]
			cursor := " "
			if i == m.cursor && m.mode == modeTimeline {
				cursor = ">"
			}
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
		if len(m.entries) > end {
			remaining := len(m.entries) - end
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
		b.WriteString("\nenter confirm, esc cancel\n")
	} else {
		b.WriteString("\nup/down move, enter assign, q quit\n")
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
	visible := m.timelineRows()
	if visible <= 0 {
		m.offset = 0
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
	maxOffset := maxInt(0, len(m.entries)-visible)
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
}

func (m AppModel) visibleRange() (int, int) {
	visible := m.timelineRows()
	start := minInt(m.offset, maxInt(0, len(m.entries)))
	end := minInt(len(m.entries), start+visible)
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
	cols := timelineColWidths{Cursor: 1, Time: 11, Status: 6, Project: 6, Description: 8}
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
