package domain

// Schedule is one published hourly-outage schedule. Date (YYYY-MM-DD) and
// Updated (YYYY-MM-DDTHH:MM) sort lexically.
type Schedule struct {
	Date    string
	Updated string
	Lines   []string
}

// LatestFor returns the most recently updated schedule for a day, or nil.
func LatestFor(schedules []Schedule, date string) *Schedule {
	var best *Schedule
	for i := range schedules {
		s := &schedules[i]
		if s.Date != date {
			continue
		}
		if best == nil || s.Updated > best.Updated {
			best = s
		}
	}
	return best
}
