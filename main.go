package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"

	"flight-tracker-slack/scraps"
	"flight-tracker-slack/slack"
	structs "flight-tracker-slack/types"
)

type Bot struct {
	SlackToken string
	Db         *sql.DB
}

type TrackedFlight struct {
	FlightID             string
	ChannelID            string
	DateDeparture        time.Time
	NotifiedPreDeparture bool
	NotifiedTakeoff      bool
	LastCruiseNotif      time.Time
	NotifiedLanding      bool
}

func main() {
	godotenv.Load()
	db := initDB("flights.db")
	bot := &Bot{
		SlackToken: os.Getenv("SLACK_BOT_TOKEN"),
		Db:         db,
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := chi.NewRouter()
	r.Post("/api/track", func(w http.ResponseWriter, r *http.Request) {
		slack.AddFlightHandler(w, r, bot.SlackToken, bot.Db)
	})
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("flight tracker running"))
	})
	go http.ListenAndServe(":"+port, r)
	fmt.Println("Bot is running on port", port)

	bot.Run()
}

func initDB(path string) *sql.DB {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		panic(err)
	}

	createTable := `
	CREATE TABLE IF NOT EXISTS tracked_flights (
		flight_id TEXT NOT NULL,
		date_departure TIMESTAMP NOT NULL,
		channel_id TEXT NOT NULL,
		notified_pre_departure BOOLEAN DEFAULT 0,
		notified_takeoff BOOLEAN DEFAULT 0,
		last_cruise_notif TIMESTAMP,
		notified_landing BOOLEAN DEFAULT 0,
		PRIMARY KEY (flight_id, date_departure)
	);
	`
	_, err = db.Exec(createTable)
	if err != nil {
		panic(err)
	}
	return db
}

func (b *Bot) Run() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		b.pollFlights()
	}
}

func (b *Bot) pollFlights() {
	rows, err := b.Db.Query("SELECT flight_id, channel_id, date_departure, notified_pre_departure, notified_takeoff, last_cruise_notif, notified_landing FROM tracked_flights")
	if err != nil {
		fmt.Println("Error querying tracked flights:", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var f TrackedFlight
		var lastCruise sql.NullTime
		if err := rows.Scan(&f.FlightID, &f.ChannelID, &f.DateDeparture, &f.NotifiedPreDeparture, &f.NotifiedTakeoff, &lastCruise, &f.NotifiedLanding); err != nil {
			fmt.Println(err)
			continue
		}
		if lastCruise.Valid {
			f.LastCruiseNotif = lastCruise.Time
		}
		b.checkAndNotifyFlight(f)
	}
}

func (b *Bot) checkAndNotifyFlight(f TrackedFlight) {
	now := time.Now().UTC()
	if f.DateDeparture.Before(now.Add(-2 * time.Hour)) {
		return
	}

	data := fetchFlightData(f)
	if data.FlightStatus == "" && data.FlightPlan.Route == "" {
		return
	}

	schedule := data.GetSchedule()
	diff := schedule.DepartureScheduled.Sub(now)

	if !f.NotifiedPreDeparture && diff <= 30*time.Minute && diff > 0 {
		sendSimpleSlack(b, f, "Flight departing soon!")
		b.Db.Exec("UPDATE tracked_flights SET notified_pre_departure = 1 WHERE flight_id = ? AND date_departure = ?", f.FlightID, f.DateDeparture)
	}

	if !f.NotifiedTakeoff && data.FlightStatus == "departed" {
		sendSimpleSlack(b, f, "Flight has taken off!")
		b.Db.Exec("UPDATE tracked_flights SET notified_takeoff = 1 WHERE flight_id = ? AND date_departure = ?", f.FlightID, f.DateDeparture)
	}

	if !f.NotifiedLanding && data.FlightStatus == "landed" {
		sendSimpleSlack(b, f, "Flight has landed!")
		b.Db.Exec("UPDATE tracked_flights SET notified_landing = 1 WHERE flight_id = ? AND date_departure = ?", f.FlightID, f.DateDeparture)
		// remove flight from tracking after landing
		b.Db.Exec("DELETE FROM tracked_flights WHERE flight_id = ? AND date_departure = ?", f.FlightID, f.DateDeparture)
		return
	}

	if data.FlightStatus == "enroute" && now.Sub(f.LastCruiseNotif) >= 2*time.Hour {
		sendSimpleSlack(b, f, "Flight is cruising")
		b.Db.Exec("UPDATE tracked_flights SET last_cruise_notif = ? WHERE flight_id = ? AND date_departure = ?", now, f.FlightID, f.DateDeparture)
	}
}

func fetchFlightData(f TrackedFlight) structs.FlightDetail {
	wrapper, err := scraps.GetFlightInfo(f.FlightID)
	if err != nil {
		fmt.Println("Error fetching flight info:", err)
		return structs.FlightDetail{}
	}

	for _, v := range wrapper.Flights {
		return v
	}
	return structs.FlightDetail{}
}

func sendSimpleSlack(b *Bot, f TrackedFlight, msg string) {
	blocks := []any{
		map[string]any{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": msg,
			},
		},
	}
	err := slack.SendSlackMessage(f.ChannelID, b.SlackToken, "", blocks)
	if err != nil {
		fmt.Println("Slack error:", err)
	}
}
