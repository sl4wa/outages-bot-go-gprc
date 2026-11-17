package domain

type Outage struct {
	StartDate  string
	EndDate    string
	City       string
	StreetID   int32
	StreetName string
	Buildings  []string
	Comment    string
}
