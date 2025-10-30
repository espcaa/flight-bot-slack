package main

import (
	"flight-tracker-slack/slack"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

type Bot struct {
	SlackToken string
}

func main() {

	bot := newBot()

	godotenv.Load()
	var port = os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// run a webserver for slack commands

	r := chi.NewRouter()
	r.Post("/api/track", func(w http.ResponseWriter, r *http.Request) {
		slack.AddFlightHandler(w, r, bot.SlackToken)
	})
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hiii ^-^"))
	})
	http.ListenAndServe(":"+port, r)
	bot.Run()
}

func newBot() Bot {
	return Bot{
		SlackToken: os.Getenv("SLACK_BOT_TOKEN"),
	}
}

func (b *Bot) Run() {

}
