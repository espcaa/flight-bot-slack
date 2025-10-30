package structs

import (
	"time"
)

type FlightDataWrapper struct {
	Flights map[string]FlightDetail `json:"flights"`
}

type FlightDetail struct {
	Aircraft           AircraftDetail `json:"aircraft"`
	Airline            AirlineDetail  `json:"airline"`
	Altitude           int            `json:"altitude"`
	Destination        AirportDetail  `json:"destination"`
	Distance           DistanceDetail `json:"distance"`
	FlightPlan         FlightPlan     `json:"flightPlan"`
	FlightStatus       string         `json:"flightStatus"`
	GateArrivalTimes   GateTimes      `json:"gateArrivalTimes"`
	GateDepartureTimes GateTimes      `json:"gateDepartureTimes"`
	LandingTimes       GateTimes      `json:"landingTimes"`
	Origin             AirportDetail  `json:"origin"`
	TakeoffTimes       GateTimes      `json:"takeoffTimes"`
	Groundspeed        int            `json:"groundspeed"`
	Heading            int            `json:"heading"`
	Timestamp          int64          `json:"timestamp"`
	Track              []TrackPoint   `json:"track"`
}

type TrackPoint struct {
	Timestamp int64      `json:"timestamp"`
	Coord     [2]float64 `json:"coord"`
	Alt       int        `json:"alt"`
	Gs        int        `json:"gs"`
	Type      string     `json:"type"`
	Isolated  bool       `json:"isolated"`
}

type AircraftDetail struct {
	FriendlyType string `json:"friendlyType"`
	Type         string `json:"type"`
}

type AirlineDetail struct {
	FullName  string `json:"fullName"`
	Callsign  string `json:"callsign"`
	Iata      string `json:"iata"`
	Icao      string `json:"icao"`
	ShortName string `json:"shortName"`
}

type AirportDetail struct {
	TZ               string `json:"TZ"`
	FriendlyLocation string `json:"friendlyLocation"`
	FriendlyName     string `json:"friendlyName"`
	Gate             string `json:"gate"`
	Iata             string `json:"iata"`
	Icao             string `json:"icao"`
	Terminal         string `json:"terminal"`
	Delays           []struct {
		Reason string `json:"reason"`
		Time   string `json:"time"`
		Type   string `json:"type"`
	} `json:"delays"`
}

type DistanceDetail struct {
	Actual    *int `json:"actual"`
	Elapsed   int  `json:"elapsed"`
	Remaining int  `json:"remaining"`
}

type GateTimes struct {
	Actual    *int64 `json:"actual"`
	Estimated *int64 `json:"estimated"`
	Scheduled int64  `json:"scheduled"`
}

type FlightPlan struct {
	Altitude        int    `json:"altitude"`
	Departure       int64  `json:"departure"`
	DirectDistance  int    `json:"directDistance"`
	PlannedDistance int    `json:"plannedDistance"`
	Route           string `json:"route"`
	Speed           int    `json:"speed"`
	Ete             int    `json:"ete"`
}

type PerformanceDetail struct {
	ArrivalDelay   int `json:"arrival"`
	DepartureDelay int `json:"departure"`
}

type FlightSchedule struct {
	DepartureScheduled time.Time
	DepartureActual    time.Time
	ArrivalScheduled   time.Time
	ArrivalEstimated   time.Time
}

func (g GateTimes) ToTime(t *int64) time.Time {
	if t != nil {
		return time.Unix(*t, 0)
	}
	return time.Time{}
}

func (fd *FlightDetail) GetSchedule() FlightSchedule {
	return FlightSchedule{
		DepartureScheduled: fd.GateDepartureTimes.ToTime(&fd.GateDepartureTimes.Scheduled),
		DepartureActual:    fd.GateDepartureTimes.ToTime(fd.GateDepartureTimes.Actual),
		ArrivalScheduled:   fd.GateArrivalTimes.ToTime(&fd.GateArrivalTimes.Scheduled),
		ArrivalEstimated:   fd.GateArrivalTimes.ToTime(fd.GateArrivalTimes.Estimated),
	}
}
