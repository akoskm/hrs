// Package model defines the core domain types for hrs.
package model

import (
	"encoding/json"
	"time"
)

// ── Clients ─────────────────────────────────────────────────

type Client struct {
	ID         string     `json:"id"          db:"id"`
	Name       string     `json:"name"        db:"name"`
	Contact    *string    `json:"contact"     db:"contact"`
	ArchivedAt *time.Time `json:"archived_at" db:"archived_at"`
	CreatedAt  time.Time  `json:"created_at"  db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"  db:"updated_at"`
}

// ── Projects ────────────────────────────────────────────────

type Currency string

const (
	CurrencyHUF Currency = "HUF"
	CurrencyCHF Currency = "CHF"
	CurrencyEUR Currency = "EUR"
	CurrencyUSD Currency = "USD"
)

type Project struct {
	ID              string     `json:"id"               db:"id"`
	ClientID        *string    `json:"client_id"        db:"client_id"`
	Name            string     `json:"name"             db:"name"`
	Code            *string    `json:"code"             db:"code"`
	HourlyRate      int        `json:"hourly_rate"      db:"hourly_rate"`
	Currency        Currency   `json:"currency"         db:"currency"`
	BillableDefault bool       `json:"billable_default" db:"billable_default"`
	Color           *string    `json:"color"            db:"color"`
	ArchivedAt      *time.Time `json:"archived_at"      db:"archived_at"`
	CreatedAt       time.Time  `json:"created_at"       db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"       db:"updated_at"`
}

func (p Project) RateAsFloat() float64 { return float64(p.HourlyRate) / 100.0 }

// ── Tasks ───────────────────────────────────────────────────

type Task struct {
	ID         string     `json:"id"          db:"id"`
	ProjectID  string     `json:"project_id"  db:"project_id"`
	Name       string     `json:"name"        db:"name"`
	Code       *string    `json:"code"        db:"code"`
	Done       bool       `json:"done"        db:"done"`
	ArchivedAt *time.Time `json:"archived_at" db:"archived_at"`
	CreatedAt  time.Time  `json:"created_at"  db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"  db:"updated_at"`
}

// ── Time Entries ────────────────────────────────────────────

type Status string

const (
	StatusDraft     Status = "draft"
	StatusConfirmed Status = "confirmed"
)

type ActivitySlot struct {
	SlotTime  time.Time
	Operator  string
	Cwd       string
	MsgCount  int
	FirstText string
}

type TimeEntry struct {
	ID           string     `json:"id"            db:"id"`
	ProjectID    *string    `json:"project_id"    db:"project_id"`
	TaskID       *string    `json:"task_id"       db:"task_id"`
	Description  *string    `json:"description"   db:"description"`
	StartedAt    time.Time  `json:"started_at"    db:"started_at"`
	EndedAt      *time.Time `json:"ended_at"      db:"ended_at"`
	DurationSecs *int       `json:"duration_secs" db:"duration_secs"`
	Billable     bool       `json:"billable"      db:"billable"`
	Status       Status     `json:"status"        db:"status"`
	Operator     string     `json:"operator"      db:"operator"`
	SourceRef    *string    `json:"source_ref"    db:"source_ref"`
	Worktree     *string    `json:"worktree"      db:"worktree"`
	GitBranch    *string    `json:"git_branch"    db:"git_branch"`
	Cwd          *string    `json:"cwd"           db:"cwd"`
	Metadata     *string    `json:"metadata"      db:"metadata"`
	DeletedAt    *time.Time `json:"deleted_at"    db:"deleted_at"`
	CreatedAt    time.Time  `json:"created_at"    db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"    db:"updated_at"`
}

func (te TimeEntry) IsRunning() bool { return te.EndedAt == nil }

func (te TimeEntry) IsDraft() bool { return te.Status == StatusDraft }

func (te TimeEntry) Duration() time.Duration {
	if te.DurationSecs != nil {
		return time.Duration(*te.DurationSecs) * time.Second
	}
	if te.EndedAt != nil {
		return te.EndedAt.Sub(te.StartedAt)
	}
	return time.Since(te.StartedAt)
}

func (te TimeEntry) ParsedMetadata() map[string]any {
	if te.Metadata == nil || *te.Metadata == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(*te.Metadata), &m); err != nil {
		return nil
	}
	return m
}

// ── Labels ──────────────────────────────────────────────────

type Label struct {
	ID        string    `json:"id"         db:"id"`
	Name      string    `json:"name"       db:"name"`
	Color     *string   `json:"color"      db:"color"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ── Import Log ──────────────────────────────────────────────

type ImportLog struct {
	ID             string    `json:"id"              db:"id"`
	Source         string    `json:"source"          db:"source"`
	SourcePath     *string   `json:"source_path"     db:"source_path"`
	SessionID      string    `json:"session_id"      db:"session_id"`
	EntriesCreated int       `json:"entries_created" db:"entries_created"`
	ImportedAt     time.Time `json:"imported_at"     db:"imported_at"`
}

// ── Project Paths ───────────────────────────────────────────

type ProjectPath struct {
	ID        string    `json:"id"         db:"id"`
	ProjectID string    `json:"project_id" db:"project_id"`
	Path      string    `json:"path"       db:"path"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ── Aggregation Types ───────────────────────────────────────

type ProjectSummary struct {
	ProjectID    string   `json:"project_id"    db:"project_id"`
	ProjectName  string   `json:"project_name"  db:"project_name"`
	ProjectCode  *string  `json:"project_code"  db:"project_code"`
	Currency     Currency `json:"currency"      db:"currency"`
	HourlyRate   int      `json:"hourly_rate"   db:"hourly_rate"`
	TotalSecs    int      `json:"total_secs"    db:"total_secs"`
	BillableSecs int      `json:"billable_secs" db:"billable_secs"`
}

func (ps ProjectSummary) TotalHours() float64    { return float64(ps.TotalSecs) / 3600.0 }
func (ps ProjectSummary) BillableHours() float64 { return float64(ps.BillableSecs) / 3600.0 }
func (ps ProjectSummary) Earnings() float64 {
	return ps.BillableHours() * float64(ps.HourlyRate) / 100.0
}

type DailySummary struct {
	Date         string `json:"date"          db:"date"`
	TotalSecs    int    `json:"total_secs"    db:"total_secs"`
	BillableSecs int    `json:"billable_secs" db:"billable_secs"`
	HumanSecs    int    `json:"human_secs"    db:"human_secs"`
	AgentSecs    int    `json:"agent_secs"    db:"agent_secs"`
}

func (ds DailySummary) TotalHours() float64 { return float64(ds.TotalSecs) / 3600.0 }
func (ds DailySummary) HumanHours() float64 { return float64(ds.HumanSecs) / 3600.0 }
func (ds DailySummary) AgentHours() float64 { return float64(ds.AgentSecs) / 3600.0 }

type OperatorSummary struct {
	Operator   string `json:"operator"    db:"operator"`
	EntryCount int    `json:"entry_count" db:"entry_count"`
	TotalSecs  int    `json:"total_secs"  db:"total_secs"`
}

func (o OperatorSummary) TotalHours() float64 { return float64(o.TotalSecs) / 3600.0 }

type LabelSummary struct {
	LabelID   string  `json:"label_id"   db:"label_id"`
	LabelName string  `json:"label_name" db:"label_name"`
	Color     *string `json:"color"      db:"color"`
	TotalSecs int     `json:"total_secs" db:"total_secs"`
}

func (ls LabelSummary) TotalHours() float64 { return float64(ls.TotalSecs) / 3600.0 }

// ── Rich Query Results ──────────────────────────────────────

type TimeEntryDetail struct {
	TimeEntry
	ProjectName  string   `json:"project_name" db:"project_name"`
	ProjectCode  *string  `json:"project_code" db:"project_code"`
	ProjectColor *string  `json:"project_color" db:"project_color"`
	TaskName     *string  `json:"task_name"    db:"task_name"`
	ClientName   *string  `json:"client_name"  db:"client_name"`
	Labels       []string `json:"labels"`
}
