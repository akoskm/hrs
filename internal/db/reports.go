package db

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/akoskm/hrs/internal/model"
)

const unassignedProjectName = "unassigned"
const unassignedProjectKey = "__unassigned__"

type ReportRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type ReportSummary struct {
	TotalSecs        int `json:"total_secs"`
	BillableSecs     int `json:"billable_secs"`
	NonBillableSecs  int `json:"non_billable_secs"`
	ActiveDays       int `json:"active_days"`
	AverageDailySecs int `json:"average_daily_secs"`
}

type ReportProjectRow struct {
	ProjectName     string  `json:"project_name"`
	ProjectCode     *string `json:"project_code,omitempty"`
	Currency        string  `json:"currency"`
	HourlyRate      int     `json:"hourly_rate"`
	TotalSecs       int     `json:"total_secs"`
	BillableSecs    int     `json:"billable_secs"`
	NonBillableSecs int     `json:"non_billable_secs"`
}

type ReportDayRow struct {
	Date            string `json:"date"`
	TotalSecs       int    `json:"total_secs"`
	BillableSecs    int    `json:"billable_secs"`
	NonBillableSecs int    `json:"non_billable_secs"`
}

type ReportResult struct {
	Range    ReportRange        `json:"range"`
	Summary  ReportSummary      `json:"summary"`
	Projects []ReportProjectRow `json:"projects"`
	Days     []ReportDayRow     `json:"days"`
}

func (s *Store) RangeReport(ctx context.Context, start, end time.Time) (ReportResult, error) {
	entries, err := s.listEntriesOverlappingRange(ctx, start, end)
	if err != nil {
		return ReportResult{}, err
	}
	projectByID := map[string]model.Project{}

	result := ReportResult{
		Range: ReportRange{
			From: start.In(time.Local).Format("2006-01-02"),
			To:   end.Add(-time.Nanosecond).In(time.Local).Format("2006-01-02"),
		},
		Projects: []ReportProjectRow{},
		Days:     []ReportDayRow{},
	}

	projects := map[string]*ReportProjectRow{}
	days := map[string]*ReportDayRow{}
	activeDays := map[string]struct{}{}

	for _, entry := range entries {
		segmentStart := maxTime(entry.StartedAt, start)
		segmentEnd := minTime(reportEntryEnd(entry), end)
		if !segmentEnd.After(segmentStart) {
			continue
		}

		projectName := entry.ProjectName
		projectKey := unassignedProjectKey
		if entry.ProjectID != nil {
			projectKey = *entry.ProjectID
		}
		if projectName == "" {
			projectName = unassignedProjectName
		}
		project, ok := projects[projectKey]
		if !ok {
			project = &ReportProjectRow{ProjectName: projectName, ProjectCode: entry.ProjectCode, Currency: string(model.CurrencyEUR)}
			if entry.ProjectID != nil {
				item, ok := projectByID[*entry.ProjectID]
				if !ok {
					item, err = s.ProjectByID(ctx, *entry.ProjectID)
					if err != nil && err != sql.ErrNoRows {
						return ReportResult{}, err
					}
					if err == nil {
						projectByID[*entry.ProjectID] = item
					}
				}
				if err == nil {
					project.ProjectCode = item.Code
					project.Currency = string(item.Currency)
					project.HourlyRate = item.HourlyRate
				}
			}
			projects[projectKey] = project
		}
		if entry.ProjectCode != nil {
			project.ProjectCode = entry.ProjectCode
		}

		for _, segment := range splitReportSegmentsByDay(segmentStart, segmentEnd) {
			duration := int(segment.end.Sub(segment.start).Seconds())
			project.TotalSecs += duration
			if entry.Billable {
				project.BillableSecs += duration
			} else {
				project.NonBillableSecs += duration
			}

			dayKey := segment.start.In(time.Local).Format("2006-01-02")
			day, ok := days[dayKey]
			if !ok {
				day = &ReportDayRow{Date: dayKey}
				days[dayKey] = day
			}
			day.TotalSecs += duration
			if entry.Billable {
				day.BillableSecs += duration
			} else {
				day.NonBillableSecs += duration
			}
			activeDays[dayKey] = struct{}{}
			result.Summary.TotalSecs += duration
			if entry.Billable {
				result.Summary.BillableSecs += duration
			} else {
				result.Summary.NonBillableSecs += duration
			}
		}
	}

	result.Summary.ActiveDays = len(activeDays)
	if result.Summary.ActiveDays > 0 {
		result.Summary.AverageDailySecs = result.Summary.TotalSecs / result.Summary.ActiveDays
	}

	for _, project := range projects {
		result.Projects = append(result.Projects, *project)
	}
	for _, day := range days {
		result.Days = append(result.Days, *day)
	}
	sort.Slice(result.Projects, func(i, j int) bool {
		if result.Projects[i].TotalSecs == result.Projects[j].TotalSecs {
			return result.Projects[i].ProjectName < result.Projects[j].ProjectName
		}
		return result.Projects[i].TotalSecs > result.Projects[j].TotalSecs
	})
	sort.Slice(result.Days, func(i, j int) bool {
		return result.Days[i].Date < result.Days[j].Date
	})

	return result, nil
}

func (s *Store) listEntriesOverlappingRange(ctx context.Context, start, end time.Time) ([]model.TimeEntryDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			te.id, te.project_id, te.task_id, te.description, te.started_at, te.ended_at, te.duration_secs,
			te.billable, te.status, te.operator, te.source_ref, te.worktree, te.git_branch, te.cwd,
			te.metadata, te.deleted_at, te.created_at, te.updated_at,
			COALESCE(p.name, ''), p.code, p.color
		FROM time_entries te
		LEFT JOIN projects p ON p.id = te.project_id
		WHERE te.deleted_at IS NULL
		  AND te.started_at < ?
		  AND COALESCE(te.ended_at, te.started_at) >= ?
		ORDER BY te.started_at
	`, end.UTC().Format(timeFormat), start.UTC().Format(timeFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.TimeEntryDetail
	for rows.Next() {
		entry, err := scanTimeEntryDetail(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func reportEntryEnd(entry model.TimeEntryDetail) time.Time {
	if entry.EndedAt != nil {
		return *entry.EndedAt
	}
	if entry.DurationSecs != nil {
		return entry.StartedAt.Add(time.Duration(*entry.DurationSecs) * time.Second)
	}
	return entry.StartedAt
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

type reportSegment struct {
	start time.Time
	end   time.Time
}

func splitReportSegmentsByDay(start, end time.Time) []reportSegment {
	if !end.After(start) {
		return nil
	}
	segments := []reportSegment{}
	current := start
	for current.Before(end) {
		local := current.In(time.Local)
		nextDay := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, time.Local)
		segmentEnd := minTime(end, nextDay)
		segments = append(segments, reportSegment{start: current, end: segmentEnd})
		current = segmentEnd
	}
	return segments
}
