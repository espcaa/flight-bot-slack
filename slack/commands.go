package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func AddFlightHandler(w http.ResponseWriter, r *http.Request, slackToken string) {

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	jsonResponse := `{
        "response_type": "ephemeral",
        "text": "processing your request..."
    }`

	w.Write([]byte(jsonResponse))

	// get the command content

	// format [flight_number] [date]

	err := r.ParseForm()
	if err != nil {
		fmt.Println("Error parsing form:", err)
		return
	}

	commandText := r.FormValue("text")
	webhookURL := r.FormValue("response_url")

	var flightNumber string
	var date string

	splitIndex := -1
	for i, char := range commandText {
		if char == ' ' {
			splitIndex = i
			break
		}
	}

	if splitIndex != -1 {
		flightNumber = commandText[:splitIndex]
		date = commandText[splitIndex+1:]
	} else {
		flightNumber = commandText
		date = ""
	}

	message := fmt.Sprintf("You requested tracking for flight *%s* on date *%s*.", flightNumber, date)

	err = answerWebhook(webhookURL, message)
	if err != nil {
		fmt.Println("Error sending Slack message:", err)
		return
	}

}

func answerWebhook(webhookURL string, message string) error {
	payload := map[string]string{
		"text": message,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return nil
}
