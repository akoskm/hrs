package db

import (
	"context"
	"time"

	"github.com/akoskm/hrs/internal/model"
)

func (s *Store) UpsertActivitySlots(ctx context.Context, slots []model.ActivitySlot) error {
	for _, slot := range slots {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO activity_slots (slot_time, operator, cwd, msg_count, first_text, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(slot_time, operator) DO UPDATE SET
				msg_count = excluded.msg_count,
				cwd = COALESCE(activity_slots.cwd, excluded.cwd)
		`, slot.SlotTime.UTC().Format(timeFormat), slot.Operator, slot.Cwd, slot.MsgCount, slot.FirstText, nowUTC().Format(timeFormat))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListActivitySlotsForDay(ctx context.Context, day time.Time) ([]model.ActivitySlot, error) {
	// day is local midnight; convert to UTC range to avoid timezone date shift
	local := day.In(time.Local)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local).UTC()
	end := start.Add(24 * time.Hour)
	rows, err := s.db.QueryContext(ctx, `
		SELECT slot_time, operator, cwd, msg_count, first_text
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
		var cwd, firstText *string
		if err := rows.Scan(&slotTime, &slot.Operator, &cwd, &slot.MsgCount, &firstText); err != nil {
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
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}
