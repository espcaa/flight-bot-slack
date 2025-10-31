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
		slack.AddFlightHandler(w, r, bot.Db)
	})
	r.Post("/api/untrack", func(w http.ResponseWriter, r *http.Request) {
		slack.RemoveFlightHandler(w, r, bot.Db)
	})
	r.Post("/api/list", func(w http.ResponseWriter, r *http.Request) {
		slack.PrintAllTrackedFlights(w, r, bot.Db)
	})
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hi :3"))
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
		last_status TEXT,
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
		fmt.Println("Polling flight:", f.FlightID, "Departure:", f.DateDeparture)
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

	println("Checking flight:", f.FlightID, "Status:", data.FlightStatus, "Departs in:", diff)

	if !f.NotifiedPreDeparture && diff <= 30*time.Minute && diff > 0 {

		_, err := b.Db.Exec("UPDATE tracked_flights SET notified_pre_departure = 1 WHERE flight_id = ? AND date_departure = ?", f.FlightID, f.DateDeparture)
		if err != nil {
			sendSimpleSlack(b, f, "Error updating pre-departure notification status : "+err.Error())
		} else {
			sendSimpleSlack(b, f, fmt.Sprintf("Flight departs in %d minutes!", int(diff.Minutes())))
		}
	}

	if !f.NotifiedTakeoff && data.FlightStatus == "departed" {

		_, err := b.Db.Exec("UPDATE tracked_flights SET notified_takeoff = 1 WHERE flight_id = ? AND date_departure = ?", f.FlightID, f.DateDeparture)
		if err != nil {
			sendSimpleSlack(b, f, "Error updating takeoff notification status : "+err.Error())
		} else {
			sendSimpleSlack(b, f, "Flight has taken off!")
		}
		return
	}

	if !f.NotifiedLanding && data.FlightStatus == "landed" {
		sendSimpleSlack(b, f, "Flight has landed!")
		_, err := b.Db.Exec("UPDATE tracked_flights SET notified_landing = 1 WHERE flight_id = ? AND date_departure = ?", f.FlightID, f.DateDeparture)
		if err != nil {
			sendSimpleSlack(b, f, "Error updating landing notification status : "+err.Error())
		} else {
			sendSimpleSlack(b, f, "Flight has landed!")
		}
		// remove flight from tracking after landing
		_, err = b.Db.Exec("DELETE FROM tracked_flights WHERE flight_id = ? AND date_departure = ?", f.FlightID, f.DateDeparture)
		if err != nil {
			sendSimpleSlack(b, f, "Error removing flight from tracking : "+err.Error())
		} else {
			sendSimpleSlack(b, f, "Flight has been removed from tracking.")
		}
		return
	}

	if data.FlightStatus == "enroute" && now.Sub(f.LastCruiseNotif) >= 2*time.Hour {

		_, err = b.Db.Exec("UPDATE tracked_flights SET last_cruise_notif = ? WHERE flight_id = ? AND date_departure = ?", now, f.FlightID, f.DateDeparture)
		if err != nil {
			sendSimpleSlack(b, f, "Error updating cruise notification time : "+err.Error())
		} else {
			sendSimpleSlack(b, f, "Flight is still enroute. Cruising update.")
		}
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
