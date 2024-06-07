package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

	firebase "firebase.google.com/go/v4"

	"github.com/go-chi/chi/v5"
	"github.com/go-resty/resty/v2"
	"google.golang.org/api/option"
)

const (
	apiKey      = "L4V2B56VD6YY8KCJCJBB6DUSK"
	apiEndpoint = "https://weather.visualcrossing.com/VisualCrossingWebServices/rest/services/timeline/"
)

type WeatherResponse struct {
	Days []struct {
		Date          string  `json:"datetime"`
		Temperature   float64 `json:"temp"`
		Precipitation float64 `json:"precip"`
	} `json:"days"`
}

type QueryParams struct {
	Location  string `json:"location"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type WeatherData struct {
	Location      string  `firestore:"location"`
	Date          string  `firestore:"date"`
	Temperature   float64 `firestore:"temperature"`
	Precipitation float64 `firestore:"precipitation"`
}

func main() {
	ctx := context.Background()

	// Initialize Firebase app
	opt := option.WithCredentialsFile("sdk.json")
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		log.Fatalln(err)
	}
	client, err := app.Firestore(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	defer client.Close()

	// Create a new router
	r := chi.NewRouter()

	// Initialize Resty client
	restClient := resty.New()

	// POST method handler
	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("POST / endpoint hit")
		var param QueryParams
		if err := json.NewDecoder(r.Body).Decode(&param); err != nil {
			log.Println("Error decoding JSON:", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Printf("Received parameters: %+v\n", param)

		apiURL := fmt.Sprintf("%s%s/%s/%s", apiEndpoint, param.Location, param.StartDate, param.EndDate)
		params := url.Values{}
		params.Add("key", apiKey)
		params.Add("unitGroup", "metric")

		log.Printf("Requesting weather data from: %s\n", apiURL)

		resp, err := restClient.R().
			SetQueryParamsFromValues(params).
			SetHeader("Content-Type", "application/json").
			Get(apiURL)

		if err != nil {
			log.Println("Error making request: ", err)
			http.Error(w, "Error making request to weather API", http.StatusInternalServerError)
			return
		}

		if resp.StatusCode() != 200 {
			log.Printf("Error: %v", resp)
			http.Error(w, "Error from weather API", resp.StatusCode())
			return
		}

		var weatherData WeatherResponse
		err = json.Unmarshal(resp.Body(), &weatherData)
		if err != nil {
			log.Println("Error decoding JSON response: ", err)
			http.Error(w, "Error decoding JSON response from weather API", http.StatusInternalServerError)
			return
		}

		for _, day := range weatherData.Days {
			doc := client.Collection("weather_data").NewDoc()
			_, err := doc.Set(ctx, WeatherData{
				Location:      param.Location,
				Date:          day.Date,
				Temperature:   day.Temperature,
				Precipitation: day.Precipitation,
			})
			if err != nil {
				log.Println("Error inserting into Firestore:", err)
				http.Error(w, "Error inserting into Firestore", http.StatusInternalServerError)
				return
			}
		}

		log.Printf("Weather data: %+v\n", weatherData)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(weatherData); err != nil {
			log.Printf("Error encoding response: %v\n", err)
		}
	})

	// GET method handler
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("GET / endpoint hit")

		colRef := client.Collection("weather_data")
		docs, err := colRef.Documents(ctx).GetAll()
		if err != nil {
			log.Println("Error getting documents from Firestore:", err)
			http.Error(w, "Error getting documents from Firestore", http.StatusInternalServerError)
			return
		}

		var data []WeatherData
		for _, doc := range docs {
			var wd WeatherData
			if err := doc.DataTo(&wd); err != nil {
				log.Printf("Error decoding Firestore data: %v\n", err)
				continue
			}
			data = append(data, wd)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Printf("Error encoding response: %v\n", err)
			http.Error(w, "Error encoding response", http.StatusInternalServerError)
		}
	})

	// Serve the requests
	http.HandleFunc("/", r.ServeHTTP)

	// Required for Cloud Functions
	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
