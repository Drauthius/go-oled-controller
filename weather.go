// Copyright 2020 Albert "Drauthius" Diserholt. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"log"
	"net/http"
	"time"

	owm "github.com/briandowns/openweathermap"
)

// The type of a weather status
type WeatherType byte

// The different weather types
const (
	// Clear sky
	ClearSky = 0x00
	// Few clouds, partially sunny
	FewClouds = 0x02
	// Cloudy
	Cloudy = 0x03
	// Rainy
	Rain = 0x04
	// Thunderstorm-y
	Thunderstorm = 0x05
	// Snowy
	Snow = 0x06
	// Misty
	Mist = 0x07
)

// The type of a weather result
type WeatherResult struct {
	// The current temperature at the location
	temperature float64
	// The current weather status
	weather WeatherType
}

// Map from weather type to characters showing icons found in glcdfont.c
var WEATHER_ICONS = map[WeatherType]string{
	// Sun icon
	ClearSky: "\x03\x04",
	// Cloud+sun icon
	FewClouds: "\x05\x06",
	// Cloud icon
	Cloudy: "\x07\x08",
	// Rain icon
	Rain: "\x09\x0A",
	// Thunder icon
	Thunderstorm: "\x0B\x0C",
	// Snow icon
	Snow: "\x0D\x0E",
	// Mist icon
	Mist: "\x0F\x10",
}

// Start a loop that gets the current temperature (in the specified unit as "C", "F", or "K") and weather status at the
// specified location, with the specified API key.
func WeatherGetTemperature(apiKey string, unit string, location string, result chan WeatherResult, quit chan bool) {
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
			// Translate the icon code to a weather type.
			var icon WeatherType
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
			result <- WeatherResult{temperature: weather.Main.Temp, weather: icon}
		}

		select {
		case <-time.After(5 * time.Minute):
		case <-quit:
			return
		}
	}
}
