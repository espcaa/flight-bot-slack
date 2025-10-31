package db

import (
	"database/sql"
	"time"
)

func AddFlight(db *sql.DB, flightID string, departureDate time.Time, channelID string) error {
	query := `
	INSERT OR IGNORE INTO tracked_flights (
		flight_id,
		date_departure,
		last_status,
		notified_pre_departure,
		notified_takeoff,
		last_cruise_notif,
		notified_landing,
		channel_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(query,
		flightID,
		departureDate.UTC().Format(time.RFC3339),
		"",
		false,
		false,
		nil,
		false,
		channelID,
	)

	return err
}

func RemoveFlight(db *sql.DB, flightID string, departureDate time.Time) error {
	query := `
	DELETE FROM tracked_flights
	WHERE flight_id = ? AND date_departure = ?
	`

	_, err := db.Exec(query, flightID, departureDate)
	return err
}
