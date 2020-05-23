// Copyright 2020 Albert "Drauthius" Diserholt. All rights reserved.
// Licensed under the MIT License.

// Get weather information for a location from OpenWeatherMap.org

package main

import (
	"log"
	"net/http"
	"time"

	owm "github.com/briandowns/openweathermap"
)

type WeatherCondition byte // The type of a weather condition
// The different weather conditions
const (
	ClearSky     = 0x00 // Clear sky
	FewClouds    = 0x01 // Few clouds, partially sunny
	Cloudy       = 0x02 // Cloudy
	Rain         = 0x03 // Rainy
	Thunderstorm = 0x04 // Thunderstorm-y
	Snow         = 0x05 // Snowy
	Mist         = 0x06 // Misty
)

// The type of a weather result
type WeatherResult struct {
	Temperature float64          // The current temperature at the location
	Weather     WeatherCondition // The current weather condition
}

// Map from weather condition to characters showing icons found in glcdfont.c
var WEATHER_ICONS = map[WeatherCondition]string{
	ClearSky:     "\x03\x04", // Sun icon
	FewClouds:    "\x05\x06", // Cloud+sun icon
	Cloudy:       "\x07\x08", // Cloud icon
	Rain:         "\x09\x0A", // Rain icon
	Thunderstorm: "\x0B\x0C", // Thunder icon
	Snow:         "\x0D\x0E", // Snow icon
	Mist:         "\x0F\x10", // Mist icon
}

// Start a loop that gets the current temperature (in the specified unit as "C", "F", or "K") and weather status at the
// specified location, with the specified API key.
func WeatherStats(apiKey string, unit string, location string, result chan WeatherResult, quit chan bool) {
	defer close(result)

	weather, err := owm.NewCurrent(unit, "EN", apiKey)
	if err != nil {
		log.Println("Failed to create weather service:", err)
		return
	}

	for {
		weather.CurrentByName(location)
		if weather.Cod != 200 {
			log.Printf("Failed to get weather report: %s (%d)\n", http.StatusText(weather.Cod), weather.Cod)
		} else if len(weather.Weather) < 1 {
			log.Println("Failed to get weather report. Unknown location?")
		} else {
			// Translate the icon code to a weather condition.
			var icon WeatherCondition
			switch weather.Weather[0].Icon[:2] {
			case "01":
				icon = ClearSky
			case "02":
				icon = FewClouds
			case "03", "04":
				icon = Cloudy
			case "09", "10":
				icon = Rain
			case "11":
				icon = Thunderstorm
			case "13":
				icon = Snow
			case "50":
				icon = Mist
			}
			result <- WeatherResult{Temperature: weather.Main.Temp, Weather: icon}
		}

		select {
		case <-time.After(5 * time.Minute):
		case <-quit:
			return
		}
	}
}
