package db

import (
	"context"
	"database/sql"
	"time"
)

type WeekReportRow struct {
	ProjectName     string
	ProjectCode     *string
	Currency        string
	HourlyRate      int
	TotalSecs       int
	BillableSecs    int
	NonBillableSecs int
	HumanSecs       int
	AgentSecs       int
}

func (s *Store) WeekReport(ctx context.Context, start, end time.Time) ([]WeekReportRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			COALESCE(p.name, 'unassigned') as project_name,
			p.code,
			COALESCE(p.currency, 'EUR') as currency,
			COALESCE(p.hourly_rate, 0) as hourly_rate,
			COALESCE(SUM(COALESCE(te.duration_secs, 0)), 0) as total_secs,
			COALESCE(SUM(CASE WHEN te.billable = 1 THEN COALESCE(te.duration_secs, 0) ELSE 0 END), 0) as billable_secs,
			COALESCE(SUM(CASE WHEN te.billable = 0 THEN COALESCE(te.duration_secs, 0) ELSE 0 END), 0) as non_billable_secs,
			COALESCE(SUM(CASE WHEN te.operator = 'human' THEN COALESCE(te.duration_secs, 0) ELSE 0 END), 0) as human_secs,
			COALESCE(SUM(CASE WHEN te.operator != 'human' THEN COALESCE(te.duration_secs, 0) ELSE 0 END), 0) as agent_secs
		FROM time_entries te
		LEFT JOIN projects p ON p.id = te.project_id
		WHERE te.deleted_at IS NULL
		  AND te.started_at >= ?
		  AND te.started_at < ?
		GROUP BY COALESCE(p.id, 'unassigned')
		ORDER BY total_secs DESC, project_name ASC
	`, start.UTC().Format(timeFormat), end.UTC().Format(timeFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []WeekReportRow
	for rows.Next() {
		var item WeekReportRow
		var code sql.NullString
		if err := rows.Scan(&item.ProjectName, &code, &item.Currency, &item.HourlyRate, &item.TotalSecs, &item.BillableSecs, &item.NonBillableSecs, &item.HumanSecs, &item.AgentSecs); err != nil {
			return nil, err
		}
		item.ProjectCode = scanNullString(code)
		items = append(items, item)
	}
	return items, rows.Err()
}
