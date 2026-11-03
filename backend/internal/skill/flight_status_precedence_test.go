package skill

import (
	"testing"

	"aviation-journey-agent/backend/internal/domain"
)

func TestResolveEOOBStatus(t *testing.T) {
	for _, tc := range []struct{ schedule, remarks, want string }{
		{"scheduled_airborne", "ON_TIME", domain.FlightStatusDeparted},
		{"landed", "BOARDING", domain.FlightStatusDeparted},
		{"arrived", "DELAYED", domain.FlightStatusDeparted},
		{"diverted", "ON_TIME", domain.FlightStatusDiverted},
		{"scheduled", "DEPARTED", domain.FlightStatusDeparted},
		{"scheduled_airborne", "CANCELLED", domain.FlightStatusCancelled},
		{"scheduled", "DELAYED", domain.FlightStatusDelayed},
		{"scheduled", "ON_TIME", domain.FlightStatusOnTime},
		{"scheduled", "unrecognized", domain.FlightStatusScheduled},
	} {
		t.Run(tc.schedule+"/"+tc.remarks, func(t *testing.T) {
			if got := resolveEOOBStatus(tc.schedule, tc.remarks); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
