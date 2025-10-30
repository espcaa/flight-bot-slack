package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	channelID := r.FormValue("channel_id")

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

	err = SendSlackMessage(channelID, message, slackToken)
	if err != nil {
		fmt.Println("Error sending Slack message:", err)
		return
	}

}

func SendSlackMessage(channelID string, message string, slack_token string) error {
	slackApiURL := "https://slack.com/api/chat.postMessage"
	requestBody, err := json.Marshal(map[string]string{
		"channel": channelID,
		"text":    message,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", slackApiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+slack_token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("Slack API error: %s", string(body))
	}

	return nil
}
