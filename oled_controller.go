// Copyright 2020 Albert "Drauthius" Diserholt. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bearsh/hid"
)

// The USB vendor ID to look for.
const VENDOR_ID = 0xFC51

// The USB product ID to look for.
const PRODUCT_ID = 0x058

// The USB usage to look for (Windows only).
const USAGE = 0x0061

// The USB usage page to look for (Windows only)
const USAGE_PAGE = 0xFF60

// The USB interface number to look for (Linux only)
const INTERFACE = 1

// The character to use for drawing a horizontal bar.
const BAR_CHAR = string(0x7F)

// The character(s) to use for drawing a mail icon.
const MAIL_ICON = "\x01\x02"

// The character to use to draw the degree (°) symbol.
const DEGREES_ICON = string(0x11)

// The type of a command to the OLED controller.
type Command byte

// Commands understood by the OLED controller.
const (
	// Set up the OLED controller, and get the screen size.
	SetUp = 0x00
	// Clear the OLED screen.
	Clear = 0x01
	// Set the content of a line on the OLED screen.
	SetLine = 0x02
	// Write changed lines to the screen.
	Present = 0x03
)

// The type of a screen identifier.
type Screen byte

// Screen identifiers
const (
	// Reference for the OLED screen on the master side
	Master = 0x00
	// Reference for the OLED screen on the slave side
	Slave = 0x01
)

// Struct containing the program arguments
type Args struct {
	// Whether debugging is enabled
	debug *bool
	// The name of the disk for which to show I/O usage (Linux only)
	sysStatDisk *string
	// The path to the JSON credential file for fetching GMail information.
	gmailCredentials *string
	// The label for which to fetch the number of unread messages.
	gmailLabel *string
	// The openweather.org API key
	weatherKey *string
	// The units in which to display temperature.
	weatherUnit *string
	// The location for which to get the current temperature.
	weatherLocation *string
}

// Global argument object
var gArgs Args

// Class for OLED control
type OLEDController struct {
	// The associated HID device
	device *hid.Device
	// The number of columns and rows available on the display(s)
	columns, rows uint8
}

// Fill the content of the master screen
// The first line is the time, the second is the current layer, the third an motivational message or number of
// unread messages, and the fourth is the current temperature.
func (oled *OLEDController) DrawMaster(quit chan bool, wg *sync.WaitGroup) {
	info := []string{"", "Layer: %l", "You look great today!", ""}
	unreadMails := make(chan int64, 5)
	weatherReport := make(chan WeatherResult, 5)
	stop := make(chan bool, 2)

	defer wg.Done()
	defer oled.SendCommand(Clear, Master, nil)

	wait := 0

	if *gArgs.gmailCredentials != "" {
		go GmailGetNumUnread(*gArgs.gmailCredentials, *gArgs.gmailLabel, unreadMails, stop)
		wait++
	}

	if *gArgs.weatherKey != "" && *gArgs.weatherLocation != "" {
		go WeatherGetTemperature(*gArgs.weatherKey, *gArgs.weatherUnit, *gArgs.weatherLocation, weatherReport, stop)
		wait++
	}

	// TODO: Replace more characters, since the firmware probably only supports [a-zA-Z]
	replacer := strings.NewReplacer("ö", "o")

	for {
		info[0] = time.Now().Local().Format("Mon Jan _2 15:04:05")
		oled.DrawScreen(Master, info)

		select {
		case numUnread, more := <-unreadMails:
			if !more {
				info[2] = ""
				wait--
				if wait < 1 {
					return
				}
				continue
			}
			info[2] = fmt.Sprintf("%s%d unread emails", MAIL_ICON, numUnread)
		case weather, more := <-weatherReport:
			if !more {
				info[3] = ""
				wait--
				if wait < 1 {
					return
				}
				continue
			}
			info[3] = fmt.Sprintf("%s%d%s%s in %s",
				WEATHER_ICONS[weather.weather],
				int(math.Round(weather.temperature)),
				DEGREES_ICON,
				*gArgs.weatherUnit,
				replacer.Replace(strings.Split(*gArgs.weatherLocation, ",")[0]))
		case <-time.After(1 * time.Second):
		case <-quit:
			if wait < 1 {
				return
			} else {
				for i := 0; i < wait; i++ {
					stop <- true
				}
			}
		}
	}
}

// Fill the content of the slave screen with system statistics.
func (oled *OLEDController) DrawSlave(quit chan bool, wg *sync.WaitGroup) {
	results := make(chan []float64, 5)
	barLen := oled.columns - 7
	columns := []string{"CPU%", "Mem%", "Swap", "Disk"}

	defer wg.Done()
	defer oled.SendCommand(Clear, Slave, nil)

	go SysStatsGet(1*time.Second, results, quit)
	for {
		select {
		case values, more := <-results:
			if !more {
				return
			}

			output := make([]string, len(values))
			for i := range values {
				value := math.Min(math.Max(0.0, values[i]), 1.0)
				// Draw the label and a nice bar.
				output[i] = fmt.Sprintf("%s[%-*s]", columns[i], barLen, strings.Repeat(BAR_CHAR, int(math.Round(float64(barLen)*value))))
			}
			oled.DrawScreen(Slave, output)
		}
	}
}

// Fill the content of the specified screen with the specified content.
func (oled *OLEDController) DrawScreen(screen Screen, lines []string) {
	for i, line := range lines {
		if i > int(oled.rows) {
			log.Printf("Attempting to draw more rows than the OLED supports: %d/%d\n", i, oled.rows)
			break
		}
		if len(line) > int(oled.columns) && *gArgs.debug {
			log.Printf("Attempting to draw more columns than the OLED supports: %d/%d\n", len(line), oled.columns)
		}
		oled.SendCommand(SetLine, screen, append([]byte{byte(i)}, line...))
		time.Sleep(10 * time.Millisecond) // Ensure that the command gets handled properly.
	}
	oled.SendCommand(Present, screen, nil)
}

// Send a command to the OLED controller.
func (oled *OLEDController) SendCommand(cmd Command, screen Screen, data []byte) bool {
	buf := make([]byte, 32)

	// Special sequence that will bypass VIA, if enabled.
	buf[0] = 0x02
	buf[1] = 0x00

	// Third byte is the command.
	buf[2] = byte(cmd)
	// Fourth byte is the screen index.
	buf[3] = byte(screen)

	// Remaining bytes are command-specific.
	if data != nil {
		copy(buf[4:32], data)
	}

	_, err := oled.device.Write(buf)
	if err != nil {
		log.Println("Failed to write to device:", err)
		return false
	}
	if *gArgs.debug {
		log.Println(">", buf[:])
	}

	return true
}

// Read a response from the OLED controller.
func (oled *OLEDController) ReadResponse() ([]byte, error) {
	buf := make([]byte, 32)
	size, err := oled.device.ReadTimeout(buf, 500)
	if err != nil {
		log.Println("Failed to read from device:", err)
		return nil, err
	} else if size < 1 {
		return nil, nil
	}

	if *gArgs.debug {
		log.Println("<", buf[:size])
	}
	if buf[0] != 0 {
		log.Printf("Command 0x%02X failed with error 0x%02X.", buf[2], buf[0])
		return nil, nil
	}
	return buf[4:], nil
}

// Loop setting up and filling the OLED screens.
func (oled *OLEDController) Run() {
	defer oled.device.Close()

	if err := oled.device.SetNonblocking(false); err != nil {
		log.Println("Failed to set the device blocking.")
		return
	}

	// Start by setting up
	oled.SendCommand(SetUp, Master, nil)
	resp, _ := oled.ReadResponse()
	if resp == nil {
		log.Println("Set up failed.")
		return
	}

	oled.columns = resp[0]
	oled.rows = resp[1]

	if *gArgs.debug {
		log.Printf("OLED size %dx%d\n", oled.columns, oled.rows)
	}
	if oled.columns < 1 || oled.rows < 1 {
		log.Println("Failed to get screen size from set up.")
		return
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	quit := make(chan bool, 5)

	wg.Add(1)
	// Read loop. Currently a bit silly, since the firmware doesn't send things of its own volition.
	go func() {
		defer wg.Done()
		for {
			select {
			case <-quit:
				return
			default:
				if _, err := oled.ReadResponse(); err != nil {
					// Read error. Device is probably unreachable.
					sigs <- syscall.SIGHUP
					return
				}
			}
		}
	}()

	wg.Add(1)
	go oled.DrawMaster(quit, &wg)

	wg.Add(1)
	go oled.DrawSlave(quit, &wg)

	// Wait for signal
	sig := <-sigs

	log.Println("Stopping due to", sig)

	for i := 0; i < 3; i++ {
		quit <- true
	}
	wg.Wait()

	if err := oled.device.SetNonblocking(true); err == nil {
		// Consume any lingering messages
		for {
			if resp, _ := oled.ReadResponse(); resp == nil {
				break
			}
		}
	}

	if sig == syscall.SIGHUP {
		return
	} else {
		os.Exit(0)
	}
}

// Main function, which handles flags and looks for the correct USB HID device.
func main() {
	log.SetPrefix("oled_controller ")
	log.Println("Started.")

	gArgs.debug = flag.Bool("debug", false, "Whether debug output should be produced")

	if runtime.GOOS == "linux" {
		gArgs.sysStatDisk = flag.String("sysstat-disk", "sda", "Which disk to monitor for I/O usage")
	}

	gArgs.gmailCredentials = flag.String("gmail-credentials", "", "Path to JSON credential file for GMail access")
	gArgs.gmailLabel = flag.String("gmail-label", "INBOX", "For which label to count unread messages")

	gArgs.weatherKey = flag.String("weather-api-key", "", "API key to openweather.org")
	gArgs.weatherUnit = flag.String("weather-unit", "C", "Temperature unit to use (C/F/K)")
	gArgs.weatherLocation = flag.String("weather-location", "", "The location to get the current weather")

	flag.Parse()

	for {
		for _, devInfo := range hid.Enumerate(VENDOR_ID, PRODUCT_ID) {
			found := false
			if runtime.GOOS == "windows" {
				found = devInfo.Usage == USAGE && devInfo.UsagePage == USAGE_PAGE
			} else {
				// FIXME: This check is weak, and will match a keyboard without raw HID enabled...
				//        Usage and UsagePage are only supported on Windows/Mac.
				found = devInfo.Interface == INTERFACE
			}

			if found {
				log.Println("Found device at:", devInfo.Path, devInfo.Usage, devInfo.UsagePage)
				device, err := devInfo.Open()
				if err != nil {
					log.Println("Failed to open device:", err)
				} else {
					oled := OLEDController{device: device}
					oled.Run()
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
}
