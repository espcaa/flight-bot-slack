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
	db, err := sql.Open("sqlite", "file:"+path+"?_busy_timeout=5000")
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
	b.pollFlights()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		b.pollFlights()
	}
}

func (b *Bot) pollFlights() {

	fmt.Println("Polling tracked flights...")

	rows, err := b.Db.Query("SELECT flight_id, channel_id, date_departure, notified_pre_departure, notified_takeoff, last_cruise_notif, notified_landing FROM tracked_flights")
	if err != nil {
		fmt.Println("Error querying tracked flights:", err)
		return
	}

	var flights []TrackedFlight
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
		flights = append(flights, f)
		fmt.Printf("Tracked flight: %s departing at %s\n", f.FlightID, f.DateDeparture.UTC().Format(time.RFC3339))
	}

	rows.Close()

	var updates []FlightUpdate

	for _, f := range flights {

		data := fetchFlightData(f)
		if data.Airline.FullName == "" {
			continue
		}

		now := time.Now().UTC()
		diff := f.DateDeparture.Sub(now)

		switch {
		case !f.NotifiedPreDeparture && diff <= 30*time.Minute && diff > 0:
			updates = append(updates, FlightUpdate{
				Flight: f,
				Type:   PreDeparture,
				Msg:    fmt.Sprintf("Flight departs in %d minutes!", int(diff.Minutes())),
			})
		case !f.NotifiedTakeoff && data.FlightStatus == "departed":
			updates = append(updates, FlightUpdate{
				Flight: f,
				Type:   Takeoff,
				Msg:    "Flight has taken off!",
			})
		case !f.NotifiedLanding && data.FlightStatus == "landed":
			updates = append(updates, FlightUpdate{
				Flight: f,
				Type:   Landing,
				Msg:    "Flight has landed!",
			})
		case data.FlightStatus == "enroute" && now.Sub(f.LastCruiseNotif) >= 2*time.Hour:
			updates = append(updates, FlightUpdate{
				Flight: f,
				Type:   Cruise,
				Msg:    "Flight is still enroute. Cruising update.",
			})
		}

		fmt.Printf("Checked flight %s: status=%s\n", f.FlightID, data.FlightStatus)
	}

	for _, update := range updates {
		fmt.Printf("Sending update for flight %s: type=%d\n", update.Flight.FlightID, update.Type)
		sendSimpleSlack(b, update.Flight, update.Msg)
		b.updateFlightStatus(update)
	}
}

func (b *Bot) updateFlightStatus(update FlightUpdate) {
	var query string
	args := []any{}

	switch update.Type {
	case PreDeparture:
		query = "UPDATE tracked_flights SET notified_pre_departure = 1 WHERE flight_id = ? AND date_departure = ?"
		args = []any{update.Flight.FlightID, update.Flight.DateDeparture}
	case Takeoff:
		query = "UPDATE tracked_flights SET notified_takeoff = 1 WHERE flight_id = ? AND date_departure = ?"
		args = []any{update.Flight.FlightID, update.Flight.DateDeparture}
	case Landing:
		query = "UPDATE tracked_flights SET notified_landing = 1 WHERE flight_id = ? AND date_departure = ?"
		args = []any{update.Flight.FlightID, update.Flight.DateDeparture}
	case Cruise:
		query = "UPDATE tracked_flights SET last_cruise_notif = ? WHERE flight_id = ? AND date_departure = ?"
		args = []any{time.Now().UTC(), update.Flight.FlightID, update.Flight.DateDeparture}
	}

	_, err := b.Db.Exec(query, args...)
	if err != nil {
		fmt.Println("Error updating flight status:", err)
		return
	}

	if update.Msg != "" {
		sendSimpleSlack(b, update.Flight, update.Msg)
	}

	if update.Type == Landing {
		_, err := b.Db.Exec("DELETE FROM tracked_flights WHERE flight_id = ? AND date_departure = ?", update.Flight.FlightID, update.Flight.DateDeparture)
		if err != nil {
			fmt.Println("Error deleting landed flight:", err)
		} else {
			sendSimpleSlack(b, update.Flight, "Flight has been removed from tracking.")
		}
	}
}

type UpdateType int

const (
	PreDeparture UpdateType = iota
	Takeoff
	Landing
	Cruise
)

type FlightUpdate struct {
	Flight TrackedFlight
	Type   UpdateType
	Msg    string
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
