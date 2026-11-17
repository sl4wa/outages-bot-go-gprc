package telegram

import (
	"fmt"
	"html"
	"strings"

	"outages-bot/internal/notifier/application/core/domain"
)

func notificationText(o domain.Outage) string {
	return fmt.Sprintf("Поточні відключення:\nМісто: %s\nВулиця: %s\n<b>%s – %s</b>\nКоментар: %s\nБудинки: %s",
		html.EscapeString(o.City), html.EscapeString(o.StreetName), html.EscapeString(displayTime(o.StartDate)),
		html.EscapeString(displayTime(o.EndDate)), html.EscapeString(o.Comment), html.EscapeString(strings.Join(o.Buildings, ", ")))
}

func displayTime(value string) string {
	// Preserve the upstream local wall time rather than applying a timezone
	// conversion. The source feed normally supplies ISO 8601 timestamps.
	if day, clock, ok := strings.Cut(value, "T"); ok && len(clock) >= 5 {
		return day + " " + clock[:5]
	}
	return value
}
