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

	"flight-tracker-slack/cdn"
	"flight-tracker-slack/maps"
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
		diff := data.GetSchedule().DepartureScheduled.Sub(now)

		switch {
		case !f.NotifiedPreDeparture && diff <= 30*time.Minute && diff > 0:
			blocks := []any{
				map[string]any{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("Flight *%s* (%s → %s) is scheduled to depart in less than 30 minutes!", f.FlightID, data.Origin.Iata, data.Destination.Iata),
					},
				},
				map[string]any{
					"type": "divider",
				},
				map[string]any{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("_Terminal %s, Gate %s_", data.Origin.Terminal, data.Origin.Gate),
					},
				},
			}
			updates = append(updates, FlightUpdate{
				Flight: f,
				Type:   PreDeparture,
				Msg: slack.SlackMessage{
					Channel: f.ChannelID,
					Blocks:  blocks,
				},
			})
		case !f.NotifiedTakeoff && data.FlightStatus == "airborne":

			var delayTime = data.GetSchedule().DepartureActual.Sub(data.GetSchedule().DepartureScheduled)
			var delayNote string
			if delayTime > 0 {
				delayNote = fmt.Sprintf("\n(The flight was delayed by %s)", delayTime.Truncate(time.Minute))
			} else {
				delayNote = ""
			}

			blocks := []any{
				map[string]any{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("🛫 Flight *%s* has taken off! \n Scheduled Arrival: *%s* %s", f.FlightID, data.GetSchedule().ArrivalScheduled.Format("15:04 MST, 02 Jan 2006"), delayNote),
					},
				},
				map[string]any{
					"type": "divider",
				},
				map[string]any{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("_Aircraft : *%s* (%s)_", data.Aircraft.FriendlyType, data.Aircraft.Type),
					},
				},
			}
			updates = append(updates, FlightUpdate{
				Flight: f,
				Type:   Takeoff,
				Msg: slack.SlackMessage{
					Channel: f.ChannelID,
					Blocks:  blocks,
				},
			})
		case !f.NotifiedLanding && data.FlightStatus == "arrived":

			totalFlightTime := data.GetSchedule().ArrivalActual.Sub(data.GetSchedule().DepartureActual)

			delayTime := data.GetSchedule().ArrivalActual.Sub(data.GetSchedule().ArrivalScheduled)
			var delayNote string
			if delayTime > 0 {
				delayNote = fmt.Sprintf("\n(The flight was delayed by %s)", delayTime.Truncate(time.Minute))
			} else {
				delayNote = ""
			}

			blocks := []any{
				map[string]any{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("🛬 Flight *%s* has landed!\n Total Flight Time: *%s* %s", f.FlightID, totalFlightTime.Truncate(time.Minute), delayNote),
					},
				},
				map[string]any{
					"type": "divider",
				},
				map[string]any{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("_Arrived at Terminal %s, Gate %s_", data.Destination.Terminal, data.Destination.Gate),
					},
				},
			}
			updates = append(updates, FlightUpdate{
				Flight: f,
				Type:   Landing,
				Msg: slack.SlackMessage{
					Channel: f.ChannelID,
					Blocks:  blocks,
				},
			})
		case data.FlightStatus == "airborne" && now.Sub(f.LastCruiseNotif) >= 2*time.Hour && f.NotifiedTakeoff:

			var lastTrackPoint structs.TrackPoint
			if len(data.Track) > 0 {
				lastTrackPoint = data.Track[len(data.Track)-1]
			}
			mapImagePath, err := maps.GenerateAircraftMap(lastTrackPoint.Coord[0], lastTrackPoint.Coord[1], data.Track)
			if err != nil {
				fmt.Println("Error generating map:", err)
				continue
			}
			defer os.Remove(mapImagePath)

			flightMapURL, err := cdn.UploadFile(mapImagePath)
			if err != nil {
				fmt.Println("Error uploading map to CDN:", err)
				continue
			}

			blocks := []any{
				map[string]any{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("✈️ Flight *%s* is currently cruising at an altitude of *%d ft* with a groundspeed of *%d knots*.", f.FlightID, data.Altitude, data.Groundspeed),
					},
				},
				map[string]any{
					"type":      "image",
					"image_url": flightMapURL,
					"alt_text":  "Aircraft Position Map",
				},
				map[string]any{
					"type": "divider",
				},
				map[string]any{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("_Estimated Arrival: *%s*_", data.GetSchedule().ArrivalEstimated.Format("15:04 MST, 02 Jan 2006")),
					},
				},
			}
			updates = append(updates, FlightUpdate{
				Flight: f,
				Type:   Cruise,
				Msg: slack.SlackMessage{
					Channel: f.ChannelID,
					Blocks:  blocks,
				},
			})
		}

		fmt.Printf("Checked flight %s: status=%s\n", f.FlightID, data.FlightStatus)
	}

	for _, update := range updates {
		fmt.Printf("Sending update for flight %s: type=%d\n", update.Flight.FlightID, update.Type)
		b.updateFlightStatus(update)
	}
}

func (b *Bot) updateFlightStatus(update FlightUpdate) {
	var query string
	var args []any
	shouldSend := false

	switch update.Type {
	case PreDeparture:
		if !update.Flight.NotifiedPreDeparture {
			query = "UPDATE tracked_flights SET notified_pre_departure = 1 WHERE flight_id = ? AND date_departure = ?"
			args = []any{update.Flight.FlightID, update.Flight.DateDeparture}
			shouldSend = true
		}
	case Takeoff:
		if !update.Flight.NotifiedTakeoff {
			query = "UPDATE tracked_flights SET notified_takeoff = 1, last_cruise_notif = ? WHERE flight_id = ? AND date_departure = ?"
			args = []any{time.Now().UTC(), update.Flight.FlightID, update.Flight.DateDeparture}
			shouldSend = true
		}
	case Landing:
		if !update.Flight.NotifiedLanding {
			query = "UPDATE tracked_flights SET notified_landing = 1 WHERE flight_id = ? AND date_departure = ?"
			args = []any{update.Flight.FlightID, update.Flight.DateDeparture}
			shouldSend = true
		}
	case Cruise:
		query = "UPDATE tracked_flights SET last_cruise_notif = ? WHERE flight_id = ? AND date_departure = ?"
		args = []any{time.Now().UTC(), update.Flight.FlightID, update.Flight.DateDeparture}
		shouldSend = true
	}

	if shouldSend {
		_, err := b.Db.Exec(query, args...)
		if err != nil {
			fmt.Println("Error updating flight status:", err)
			return
		}

		err = slack.SendSlackMessageTyped(update.Msg, b.SlackToken)
		if err != nil {
			fmt.Println("Slack error:", err)
			return
		}

		// Delete landed flights after notifying
		if update.Type == Landing {
			_, err := b.Db.Exec("DELETE FROM tracked_flights WHERE flight_id = ? AND date_departure = ?", update.Flight.FlightID, update.Flight.DateDeparture)
			if err != nil {
				sendSimpleSlack(b, update.Flight, fmt.Sprintf("Error removing landed flight %s from tracking: %v", update.Flight.FlightID, err))
				fmt.Println("Error removing landed flight:", err)
			}
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
	Msg    slack.SlackMessage
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
