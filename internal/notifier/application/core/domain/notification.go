package domain

type Notification struct {
	StartDate string
	EndDate   string
	Comment   string
}

type Subscription struct {
	ChatID           int64
	StreetID         int32
	Building         string
	Generation       string
	LastNotification *Notification
}

type Outage struct {
	StartDate  string
	EndDate    string
	City       string
	StreetID   int32
	StreetName string
	Buildings  []string
	Comment    string
}

type RunResult struct {
	Subscribers  int32
	Matched      int32
	Sent         int32
	Skipped      int32
	Unsubscribed int32
}

// MatchingOutage selects at most one outage per address. Any matching receipt
// suppresses the address even when another outage appears earlier in the feed.
func MatchingOutage(s Subscription, outages []Outage) *Outage {
	var first *Outage
	for i := range outages {
		outage := &outages[i]
		if outage.StreetID != s.StreetID {
			continue
		}
		matches := false
		for _, building := range outage.Buildings {
			if building == s.Building {
				matches = true
				break
			}
		}
		if !matches {
			continue
		}
		last := s.LastNotification
		if last != nil && last.StartDate == outage.StartDate && last.EndDate == outage.EndDate && last.Comment == outage.Comment {
			return nil
		}
		if first == nil {
			first = outage
		}
	}
	return first
}
