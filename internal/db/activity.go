package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/akoskm/hrs/internal/model"
)

func (s *Store) UpsertActivitySlots(ctx context.Context, slots []model.ActivitySlot) error {
	for _, slot := range slots {
		userTexts := slot.UserTexts
		if len(userTexts) == 0 {
			userTexts = []string{}
		}
		userTextsJSON, _ := json.Marshal(userTexts)
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO activity_slots (slot_time, operator, cwd, msg_count, first_text, git_branch, user_texts, token_input, token_output, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(slot_time, operator) DO UPDATE SET
				msg_count = excluded.msg_count,
				first_text = excluded.first_text,
				cwd = COALESCE(activity_slots.cwd, excluded.cwd),
				git_branch = COALESCE(excluded.git_branch, activity_slots.git_branch),
				user_texts = excluded.user_texts,
				token_input = excluded.token_input,
				token_output = excluded.token_output
		`, slot.SlotTime.UTC().Format(timeFormat), slot.Operator, slot.Cwd, slot.MsgCount, slot.FirstText,
			slot.GitBranch, string(userTextsJSON), slot.TokenInput, slot.TokenOutput, nowUTC().Format(timeFormat))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListActivitySlotsForDay(ctx context.Context, day time.Time) ([]model.ActivitySlot, error) {
	local := day.In(time.Local)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local).UTC()
	end := start.Add(24 * time.Hour)
	rows, err := s.db.QueryContext(ctx, `
		SELECT slot_time, operator, cwd, msg_count, first_text, git_branch, user_texts, token_input, token_output
		FROM activity_slots
		WHERE slot_time >= ? AND slot_time < ?
		ORDER BY slot_time
	`, start.Format(timeFormat), end.Format(timeFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slots []model.ActivitySlot
	for rows.Next() {
		var slot model.ActivitySlot
		var slotTime string
		var cwd, firstText, gitBranch, userTextsJSON *string
		if err := rows.Scan(&slotTime, &slot.Operator, &cwd, &slot.MsgCount, &firstText, &gitBranch, &userTextsJSON, &slot.TokenInput, &slot.TokenOutput); err != nil {
			return nil, err
		}
		slot.SlotTime, err = parseTime(slotTime)
		if err != nil {
			return nil, err
		}
		if cwd != nil {
			slot.Cwd = *cwd
		}
		if firstText != nil {
			slot.FirstText = *firstText
		}
		if gitBranch != nil {
			slot.GitBranch = *gitBranch
		}
		if userTextsJSON != nil && *userTextsJSON != "" {
			_ = json.Unmarshal([]byte(*userTextsJSON), &slot.UserTexts)
		}
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}
