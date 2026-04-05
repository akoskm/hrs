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
	dayStr := day.UTC().Format("2006-01-02")
	rows, err := s.db.QueryContext(ctx, `
		SELECT slot_time, operator, cwd, msg_count, first_text
		FROM activity_slots
		WHERE date(slot_time) = ?
		ORDER BY slot_time
	`, dayStr)
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
