package scraps

import (
	"encoding/json"
	structs "flight-tracker-slack/types"
	"io"
	"net/http"
	"regexp"
	"time"
)

const (
	NmToMi  float64 = 1.15078
	NmToKm  float64 = 1.852
	KtToMph float64 = 1.15078
)

func unixToTime(timestamp int64) time.Time {
	if timestamp == 0 {
		return time.Time{}
	}
	return time.Unix(timestamp, 0).In(time.UTC)
}

var dataRegex = regexp.MustCompile(`trackpollBootstrap = (\{.*?\});`)

func GetFlightInfo(flightNumber string) (structs.FlightDataWrapper, error) {
	// create a http client with timeout

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
	}

	// make a get request
	req, err := http.NewRequest("GET", "https://fr.flightaware.com/live/flight/"+flightNumber, nil)
	if err != nil {
		return structs.FlightDataWrapper{}, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return structs.FlightDataWrapper{}, err
	}
	defer resp.Body.Close()

	// read the response body

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return structs.FlightDataWrapper{}, err
	}

	matches := dataRegex.FindSubmatch(body)
	if len(matches) < 2 {
		return structs.FlightDataWrapper{}, nil
	}

	jsonData := matches[1]

	var flightData structs.FlightDataWrapper
	err = json.Unmarshal(jsonData, &flightData)
	if err != nil {
		return structs.FlightDataWrapper{}, err
	}

	return flightData, nil
}
