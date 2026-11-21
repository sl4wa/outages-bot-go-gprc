package telegram

import (
	"context"
	"html"
	"strings"
	"time"

	"outages-bot/internal/notifier/application/core/domain"
)

func (a *Adapter) SendSchedules(ctx context.Context, chatID int64, today string, schedules []domain.Schedule) error {
	return mapError(a.client.Send(ctx, chatID, scheduleText(today, schedules)))
}

func scheduleText(today string, schedules []domain.Schedule) string {
	blocks := make([]string, 0, len(schedules))
	for _, s := range schedules {
		label := dayLabel(s.Date, today)
		lines := []string{"<b>Графік відключень на " + label + "</b>"}
		for _, line := range s.Lines {
			lines = append(lines, html.EscapeString(line))
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

func dayLabel(date, today string) string {
	day, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return date
	}
	switch base, _ := time.Parse(time.DateOnly, today); {
	case day.Equal(base):
		return "сьогодні"
	case day.Equal(base.AddDate(0, 0, 1)):
		return "завтра"
	}
	return day.Format("02.01.2006")
}
