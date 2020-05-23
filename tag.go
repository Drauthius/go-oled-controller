// Copyright 2020 Albert "Drauthius" Diserholt. All rights reserved.
// Licensed under the MIT License.

// A tag represents different content that can be shown on the screens (i.e. a "view"). Each tag is identified by a
// number, and each screen can show the content of at most one tag. Define different tags in this file to make the
// screens show different content.

package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// The tag interface
type Tag interface {
	// Function to draw content to a tag. The function will be run in a goroutine, and must close the results channel
	// upon exit. Put content to draw on the results channel, which expects lines up to area.Height.
	Draw(area Area, results chan []string, quit chan bool)
}

type GeneralInfo struct{} // Tag interface for showing general information.
type SysStats struct{}    // Tag interface for showing system status.
type GPUStats struct{}    // Tag interface for showing status of the graphics card.

// Map containing the available tags and their unique index.
// It should be kept consecutive, and starting from 1, if you wish to use the increment/decrement feature. The set_tag
// event will send the number that was pressed (e.g. KC_1 => 1).
var tags = map[uint8]Tag{
	1: &GeneralInfo{},
	2: &SysStats{},
	3: &GPUStats{},
}

// Draws some general information.
// The first line is the time, the second is the current layer, the third a motivational message or number of
// unread messages, and the fourth is the current temperature.
func (*GeneralInfo) Draw(area Area, results chan []string, quit chan bool) {
	defer close(results)

	info := []string{"", "Layer: %l", "You look great today!", ""}
	unreadMails := make(chan int64, 5)
	weatherReport := make(chan WeatherResult, 5)
	stop := make(chan bool)
	stopped := false
	wait := 0

	if *gArgs.gmailCredentials != "" {
		go GmailStats(*gArgs.gmailCredentials, *gArgs.gmailLabel, unreadMails, stop)
		wait++
	}

	var location string
	if *gArgs.weatherKey != "" && *gArgs.weatherLocation != "" {
		go WeatherStats(*gArgs.weatherKey, *gArgs.temperatureUnit, *gArgs.weatherLocation, weatherReport, stop)
		wait++

		// The firmware only supports Latin characters, without diacritics. These need to be either normalized, or
		// removed completely before drawn to the display, otherwise it just won't look right.
		isNonLatin := func(r rune) bool { return r >= 0x80 }
		t := transform.Chain(norm.NFD, transform.RemoveFunc(isNonLatin), norm.NFC)
		// Assume that the location is <city>,<country>
		location, _, _ = transform.String(t, strings.Split(*gArgs.weatherLocation, ",")[0])
	}

	for {
		info[0] = time.Now().Local().Format("Mon Jan _2 15:04:05")
		results <- info

		select {
		case numUnread, more := <-unreadMails:
			if !more {
				info[2] = ""
				wait--
				if stopped && wait < 1 {
					return
				}
				continue
			}
			info[2] = fmt.Sprintf("%s%d unread emails", MAIL_ICON, numUnread)
		case weather, more := <-weatherReport:
			if !more {
				info[3] = ""
				wait--
				if stopped && wait < 1 {
					return
				}
				continue
			}
			info[3] = fmt.Sprintf("%s%d%s%s in %s",
				WEATHER_ICONS[weather.Weather],
				int(math.Round(weather.Temperature)),
				DEGREES_ICON,
				*gArgs.temperatureUnit,
				location)
		case <-time.After(1 * time.Second):
		case <-quit:
			if wait < 1 {
				return
			} else if !stopped {
				close(stop)
			}
			stopped = true
		}
	}
}

// Draw system status as bar graphs.
// The bars are CPU, memory, swap (page file), and disk usage as percentages.
func (*SysStats) Draw(area Area, results chan []string, quit chan bool) {
	defer close(results)

	sysStat := make(chan []float64, 5)
	columns := []string{"CPU%", "Mem%", "Swap", "Disk"}

	go SystemStats(1*time.Second, sysStat, quit)
	for {
		select {
		case values, more := <-sysStat:
			if !more {
				return
			}

			output := make([]string, len(values))
			for i, value := range values {
				value := math.Min(math.Max(0.0, value), 1.0)
				if math.IsInf(value, 0) || math.IsNaN(value) {
					value = 0.0
				}
				barLen := int(area.Width) - len(columns[i]) - 2
				// Draw the label and a nice bar.
				output[i] = fmt.Sprintf("%s[%-*s]",
					columns[i],
					barLen,
					strings.Repeat(BAR_CHAR, int(math.Round(float64(barLen)*value))))
			}
			results <- output
		}
	}
}

// Draw status of the graphics card as bar graphs.
// The bars are GPU, memory, and PCIe bus utilization in percentages. It will also show the current temperature.
func (*GPUStats) Draw(area Area, results chan []string, quit chan bool) {
	defer close(results)

	gpuStats := make(chan GraphicCardResult, 5)
	columns := []string{"GPU%", "Mem%", "PCIe", FAN_ICON_2}

	go GraphicCardStats(1*time.Second, *gArgs.temperatureUnit, gpuStats, quit)
	for {
		select {
		case result, more := <-gpuStats:
			if !more {
				return
			}

			values := []float64{
				result.GPU,
				result.Memory,
				result.PCIBandwidth,
				result.FanSpeed,
			}

			output := make([]string, len(values))
			for i, value := range values {
				label := columns[i]

				// Clamp
				value := math.Min(math.Max(0.0, value), 1.0)
				if math.IsInf(value, 0) || math.IsNaN(value) {
					value = 0.0
				}

				prefix := ""
				if i == len(values)-1 { // Temperature + Fan speed
					temp := strconv.Itoa(int(math.Round(result.Temperature)))
					prefix = fmt.Sprintf("Temp:%s%s%s%s",
						temp,
						DEGREES_ICON,
						*gArgs.temperatureUnit,
						strings.Repeat(" ", 4-len(temp)))

					// Swap icon each iteration
					if columns[i] == FAN_ICON_1 {
						columns[i] = FAN_ICON_2
					} else {
						columns[i] = FAN_ICON_1
					}
				}

				barLen := int(area.Width) - len(label) - len(prefix) - 2
				// Draw the label and a nice bar.
				output[i] = fmt.Sprintf("%s%s[%-*s]",
					prefix,
					columns[i],
					barLen,
					strings.Repeat(BAR_CHAR, int(math.Round(float64(barLen)*value))))
			}
			results <- output
		}
	}
}
