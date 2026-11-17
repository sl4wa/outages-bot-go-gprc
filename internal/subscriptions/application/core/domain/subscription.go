package domain

type Street struct {
	ID   int32
	Name string
}

type Address struct {
	Street   Street
	Building string
}

type Notification struct {
	StartDate string
	EndDate   string
	Comment   string
}

type Subscription struct {
	ChatID           int64
	Address          Address
	Generation       string
	LastNotification *Notification
}

type SubscriptionGuard struct {
	Generation string
}

type StreetSearch struct {
	Streets []Street
	TooMany bool
}
