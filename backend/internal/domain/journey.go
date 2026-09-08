package domain

import "time"

type Journey struct {
	ID        string        `json:"id"`
	Flights   []FlightInput `json:"flights"`
	Passenger Passenger     `json:"passenger"`
	CreatedAt time.Time     `json:"createdAt"`
}

type FlightInput struct {
	Number string `json:"number" binding:"required"`
	Date   string `json:"date" binding:"required"`
	From   string `json:"from" binding:"required"`
	To     string `json:"to" binding:"required"`
}

type Passenger struct {
	HasBaggage bool    `json:"hasBaggage"`
	WalkSpeed  float64 `json:"walkSpeed"`
}

type Location struct {
	Latitude  float64   `json:"lat"`
	Longitude float64   `json:"lng"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updatedAt"`
}
