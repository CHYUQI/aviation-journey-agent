package domain

import "time"

type WorldState struct {
	Flight      FlightState    `json:"flight"`
	Airport     AirportState   `json:"airport"`
	Travel      TravelState    `json:"travel"`
	Passenger   PassengerState `json:"passenger"`
	Derived     DerivedState   `json:"derived"`
	Evidence    []Evidence     `json:"evidence"`
	DataQuality DataQuality    `json:"dataQuality"`
	GeneratedAt time.Time      `json:"generatedAt"`
}

type FlightState struct {
	Number             string    `json:"number"`
	Status             string    `json:"status"`
	ScheduledDeparture time.Time `json:"scheduledDeparture,omitempty"`
	EstimatedDeparture time.Time `json:"estimatedDeparture,omitempty"`
	BoardingTime       time.Time `json:"boardingTime,omitempty"`
	GateCloseTime      time.Time `json:"gateCloseTime,omitempty"`
	Gate               string    `json:"gate,omitempty"`
}

type AirportState struct {
	Code            string  `json:"code"`
	Terminal        string  `json:"terminal"`
	SecurityWaitMin int     `json:"securityWaitMin,omitempty"`
	CheckInQueueMin int     `json:"checkInQueueMin,omitempty"`
	WalkToGateMin   int     `json:"walkToGateMin,omitempty"`
	InternalGuide   []Guide `json:"internalGuide,omitempty"`
}

type Guide struct {
	From string `json:"from"`
	To   string `json:"to"`
	Min  int    `json:"min"`
	Text string `json:"text"`
}

type TravelState struct {
	DistanceKm float64   `json:"distanceKm,omitempty"`
	ETAMin     int       `json:"etaMin,omitempty"`
	Traffic    string    `json:"traffic,omitempty"`
	Location   *Location `json:"location,omitempty"`
}

type PassengerState struct {
	HasBaggage bool      `json:"hasBaggage"`
	Location   *Location `json:"location,omitempty"`
}

type DerivedState struct {
	CheckInDeadline    time.Time `json:"checkInDeadline,omitempty"`
	BagDropDeadline    time.Time `json:"bagDropDeadline,omitempty"`
	LatestDeparture    time.Time `json:"latestDeparture,omitempty"`
	AirportInternalMin int       `json:"airportInternalMin,omitempty"`
	BufferMin          int       `json:"bufferMin,omitempty"`
}

type Evidence struct {
	Field      string    `json:"field"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observedAt"`
	ExpiresAt  time.Time `json:"expiresAt,omitempty"`
	Confidence string    `json:"confidence"`
	Method     string    `json:"method,omitempty"`
}

type DataQuality struct {
	Status string   `json:"status"`
	Issues []string `json:"issues,omitempty"`
}
