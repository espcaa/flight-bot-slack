package slack

import (
	"bytes"
	sqlite "database/sql"
	"encoding/json"
	"flight-tracker-slack/db"
	"flight-tracker-slack/scraps"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

func AddFlightHandler(w http.ResponseWriter, r *http.Request, slackToken string, database *sqlite.DB) {

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

	var message string

	if !isValidFlightCode(flightNumber) {
		message = "Invalid or unknown flight code. Please provide a valid flight number (e.g., AA100)."
		err = answerWebhook(webhookURL, message, true)
		if err != nil {
			fmt.Println("Error sending Slack message:", err)
		}
		return
	}

	// parse date
	var flightDate time.Time
	if date == "" {
		flightDate = time.Now()
	} else {
		parsedDate, err := parseDate(date)
		if err != nil {
			message = "Invalid date format. Please use 'today', 'tomorrow', 'DD/MM/YYYY', 'YYYY-MM-DD', or 'DD-MM-YYYY'."
			err = answerWebhook(webhookURL, message, true)
			if err != nil {
				fmt.Println("Error sending Slack message:", err)
			}
			return
		}
		flightDate = parsedDate
	}

	message = fmt.Sprintf("Flight %s has been added for tracking on %s.", flightNumber, flightDate.Format("02 Jan 2006"))

	db.AddFlight(database, flightNumber, flightDate, r.FormValue("channel_id"))

	err = answerWebhook(webhookURL, message, false)
	if err != nil {
		fmt.Println("Error sending Slack message:", err)
	}

}

func answerWebhook(webhookURL string, message string, ephemeral bool) error {
	var responseType string
	if ephemeral {
		responseType = "ephemeral"
	} else {
		responseType = "in_channel"
	}
	payload := map[string]string{
		"text":          message,
		"response_type": responseType,
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

func isValidFlightCode(code string) bool {
	re := regexp.MustCompile(`^[A-Z]{2,3}\d{1,4}$`)
	if re.MatchString(code) == false {
		return false
	}
	// now make a request to flightaware to see if the flight exists
	var result, err = scraps.GetFlightInfo(code)
	if err != nil {
		return false
	}
	var counter int = 0
	for range result.Flights {
		counter++
	}
	if counter > 0 {
		return true
	}
	return false
}

func parseDate(input string) (time.Time, error) {
	input = strings.ToLower(strings.TrimSpace(input))
	now := time.Now()

	switch input {
	case "today":
		return now, nil
	case "tomorrow":
		return now.AddDate(0, 0, 1), nil
	}

	layouts := []string{
		"02/01/2006",
		"2006-01-02",
		"02-01-2006",
		time.RFC3339,
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, input); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid date format")
}
