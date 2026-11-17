package domain

import "errors"

var ErrNotFound = errors.New("subscription not found")

type Street struct {
	ID   int32
	Name string
}

type Address struct {
	Street   Street
	Building string
}

type Subscription struct {
	Address Address
}

type StreetSearch struct {
	Streets []Street
	TooMany bool
}

type Reply struct {
	Text           string
	Keyboard       [][]string
	RemoveKeyboard bool
}
