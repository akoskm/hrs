package tui

import (
	"context"
	"fmt"
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
			}
		case "down", "j":
			if m.mode == modeAssign {
				if m.projectCursor < len(m.projects)-1 {
					m.projectCursor++
				}
			} else if m.cursor < len(m.entries)-1 {
				m.cursor++
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
		for i, entry := range m.entries {
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
			b.WriteString(fmt.Sprintf("%s %s | %s | %s | %s\n", cursor, formatRange(entry.StartedAt, entry.EndedAt), desc, entry.Status, project))
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

func formatRange(start time.Time, end *time.Time) string {
	if end == nil {
		return start.Format("15:04") + "-..."
	}
	return start.Format("15:04") + "-" + end.Format("15:04")
}
