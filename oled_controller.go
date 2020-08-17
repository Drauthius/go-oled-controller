// Copyright 2020 Albert "Drauthius" Diserholt. All rights reserved.
// Licensed under the MIT License.

// Show programmable information on the OLED screens. This file is responsible for finding the screens, talking with
// the QMK firmware, and keeping track of the tags that are to be shown on them.
// Note that a special version of the QMK firmware is required, and a special version of glcdfont.c to proper show all
// the icons and text.

package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/bearsh/hid"
)

// Device detection constants.
const (
	VENDOR_ID  = 0xFC51 // The USB vendor ID to look for.
	PRODUCT_ID = 0x058  // The USB product ID to look for.
	USAGE      = 0x0061 // The USB usage to look for (Windows/Mac only).
	USAGE_PAGE = 0xFF60 // The USB usage page to look for (Windows/Mac only)
	INTERFACE  = 1      // The USB interface number to look for (Linux only)
)

// Struct containing the program arguments
type Args struct {
	debug            *bool   // Whether debugging is enabled
	temperatureUnit  *string // The unit in which to display temperature (C, F, or K).
	sysStatDisk      *string // The name of the disk for which to show I/O usage (Linux only)
	gmailCredentials *string // The path to the JSON credential file for fetching GMail information.
	gmailLabel       *string // The label for which to fetch the number of unread messages.
	weatherKey       *string // The openweathermap.org API key
	weatherLocation  *string // The location for which to get the current temperature.
}

// Global argument object
var gArgs Args

// Icon constants. Assumes a custom glcdfont.c to show some of the nicer icons.
const (
	BAR_CHAR     = "\x7F"     // The character to use for drawing a horizontal bar.
	MAIL_ICON    = "\x01\x02" // The character(s) to use for drawing a mail icon.
	DEGREES_ICON = "\x11"     // The character to use to draw the degree (°) symbol.
	FAN_ICON_1   = "\x12\x13" // Characters showing a fan icon, variant 1
	FAN_ICON_2   = "\x14\x15" // Characters showing a fan icon, variant 2
)

type MessageID byte // The type of a message to/from the OLED controller.
// Messages understood by the OLED controller.
const (
	CommandMsg = 0xC0
	EventMsg   = 0xC1
)

type ResultID byte // The type of a result code
// The result codes
const (
	Success = 0x00
	Failure = 0x01
)

type CommandID byte // The type of a command to the OLED controller.
// Commands understood by the OLED controller.
const (
	SetUp    = 0x00 // Set up the OLED controller, and get the screen size.
	Clear    = 0x01 // Clear an OLED screen.
	SetLine  = 0x02 // Set the content of a line on an OLED screen.
	SetChars = 0x03 // Set the content of a portion of the OLED screen.
	Present  = 0x04 // Show changed lines to a screen.
)

type EventID byte // The type of an event from the OLED controller.
// Events understood by the OLED controller.
const (
	ChangeTag    = 0x00 // Change the content of a screen.
	IncrementTag = 0x01 // Increment the tag shown on the screen by one.
	DecrementTag = 0x02 // Decrement the tag shown on the screen by one.
)

type ScreenID byte // The type of a screen identifier.
// Screen identifiers
const (
	Master = 0x00 // OLED screen on the master side
	Slave  = 0x01 // OLED screen on the slave side
)

// Structure holding a response from the OLED controller.
type Response struct {
	Success bool      // Whether the command was successful.
	Command CommandID // The command that was issued.
	Screen  ScreenID  // Which screen the command was for.
	Params  []byte    // Additional parameters sent by the firmware.
}

// Structure holding an event from the OLED controller.
type Event struct {
	Event  EventID  // The event that was issued.
	Screen ScreenID // Which screen the event is for.
	Params []byte   // Addition parameters set by the firmware.
}

// Class for OLED control
type OLEDController struct {
	Device        *hid.Device // The associated HID device
	Columns, Rows uint8       // The number of columns and rows available on the display(s)
}

// Screen size, in characters.
type Area struct {
	Width, Height uint8 // The width and height
}

// Class for screen control
type Screen struct {
	ID         ScreenID        // The screen's unique ID
	Controller *OLEDController // Reference to the OLED controller
	Tag        uint8           // Which tag to show
	Events     chan Event      // Channel to handle events
	Quit       chan bool       // Channel to handle termination
}

// Start the screen handler.
// It will run until the screen.Quit channel has been closed.
func (screen *Screen) Run(wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	defer screen.Controller.SendCommand(Clear, screen.ID, nil)

	stopped := false
	hasTag := false
	stop := make(chan bool)
	results := make(chan []string, 5)

	showTag := func(tagID uint8) {
		tag, found := tags[tagID]
		if !found {
			log.Printf("Tag %d out of range.", tagID)
		} else {
			hasTag = true
			screen.Tag = tagID
			results = make(chan []string, 5)
			go tag.Draw(Area{screen.Controller.Columns, screen.Controller.Rows}, results, stop)
		}
	}

	showTag(screen.Tag)

	for {
		select {
		case event := <-screen.Events:
			if event.Screen != screen.ID {
				log.Panicln("Received event intended for another screen:", event)
			} else if stopped {
				log.Println("Got event while shutting down:", event)
				continue
			}

			var tag uint8
			switch event.Event {
			case ChangeTag:
				tag = event.Params[0]
			case IncrementTag:
				if len(tags) < 1 {
					log.Printf("Cannot increment tag: No tags found.")
					continue
				}
				// TODO: This check doesn't account for a non-consecutive tags map, that doesn't start at 1.
				tag = screen.Tag + 1
				if _, found := tags[tag]; !found {
					tag = 1
					if _, found := tags[tag]; !found {
						log.Printf("Failed to increment tag of screen 0x%02X currently on tag %d.\n", screen.ID, screen.Tag)
						continue
					}
				}
			case DecrementTag:
				if len(tags) < 1 {
					log.Printf("Cannot increment tag: No tags found.")
					continue
				}
				// TODO: This check doesn't account for a non-consecutive tags map, that doesn't end on len(tags)
				tag = screen.Tag - 1
				if _, found := tags[tag]; !found {
					tag = uint8(len(tags))
					if _, found := tags[tag]; !found {
						log.Printf("Failed to decrement tag of screen 0x%02X currently on tag %d.\n", screen.ID, screen.Tag)
						continue
					}
				}
			}

			// Change the currently shown tag.
			if hasTag {
				stop <- true
			}
			showTag(tag)
		case lines, more := <-results:
			if !more {
				if stopped {
					return
				}
				hasTag = false
				screen.Controller.SendCommand(Clear, screen.ID, nil)
			} else {
				screen.Controller.DrawScreen(screen.ID, lines)
			}
		case <-screen.Quit:
			screen.Controller.SendCommand(Clear, screen.ID, nil)
			if hasTag {
				if !stopped {
					stop <- true
				}
			} else {
				return
			}
			stopped = true
		}
	}
}

// Draw the specified content to the specified screen.
func (oled *OLEDController) DrawScreen(screen ScreenID, lines []string) {
	for i, line := range lines {
		if i > int(oled.Rows) {
			log.Printf("Attempting to draw more rows than the OLED supports: %d/%d\n", i, oled.Rows)
			break
		}
		if len(line) > int(oled.Columns) && *gArgs.debug {
			log.Printf("Attempting to draw more columns than the OLED supports: %d/%d\n", len(line), oled.Columns)
		}
		oled.SendCommand(SetLine, screen, append([]byte{byte(i)}, line...))
		time.Sleep(10 * time.Millisecond) // Ensure that the command gets handled properly.
	}
	oled.SendCommand(Present, screen, nil)
}

// Draw over a part of the screen
// Note: Start offset is zero indexed
func (oled *OLEDController) DrawChars(screen ScreenID, start uint8, chars string) {
	oled.SendCommand(SetChars, screen, append([]byte{byte(start), byte(len(chars))}, chars...))
}

// Send a command to the OLED controller.
func (oled *OLEDController) SendCommand(cmd CommandID, screen ScreenID, data []byte) bool {
	buf := make([]byte, 32)

	buf[0] = byte(CommandMsg)
	buf[1] = byte(cmd)
	buf[2] = byte(screen)

	// Remaining bytes are command-specific.
	if data != nil {
		copy(buf[3:32], data)
	}

	_, err := oled.Device.Write(buf)
	if err != nil {
		log.Println("Failed to write to device:", err)
		return false
	}
	if *gArgs.debug {
		log.Println(">", buf[:])
	}

	return true
}

// Read a response or event from the OLED controller.
func (oled *OLEDController) ReadResponse() (interface{}, error) {
	buf := make([]byte, 32)
	size, err := oled.Device.ReadTimeout(buf, 500)
	if err != nil {
		log.Println("Failed to read from device:", err)
		return nil, err
	} else if size < 1 {
		// Timed out
		return nil, nil
	}

	if *gArgs.debug {
		log.Println("<", buf[:size])
	}
	switch buf[0] {
	case Success, Failure:
		resp := Response{
			Success: buf[0] == Success,
			Command: CommandID(buf[1]),
			Screen:  ScreenID(buf[2]),
			Params:  buf[3:],
		}

		if !resp.Success {
			log.Printf("Command 0x%02X failed with error 0x%02X.\n", resp.Command, buf[0])
			return nil, nil
		}

		return resp, nil
	case EventMsg:
		event := Event{
			Event:  EventID(buf[1]),
			Screen: ScreenID(buf[2]),
			Params: buf[3:],
		}
		return event, nil
	default:
		log.Printf("Received unknown message 0x%02X\n", buf[0])
		return nil, nil
	}
}

// Loop setting up and filling the OLED screens.
func (oled *OLEDController) Run() {
	defer oled.Device.Close()

	if err := oled.Device.SetNonblocking(false); err != nil {
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
	switch resp.(type) {
	case Response:
		oled.Columns = resp.(Response).Params[0]
		oled.Rows = resp.(Response).Params[1]
	default:
		log.Println("Wrong response for set up command.")
		return
	}

	if *gArgs.debug {
		log.Printf("OLED size %dx%d\n", oled.Columns, oled.Rows)
	}
	if oled.Columns < 1 || oled.Rows < 1 {
		log.Println("Failed to get screen size from set up.")
		return
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	var wg sync.WaitGroup
	quit := make(chan bool, 5)
	masterCtrl := make(chan Event, 1)
	slaveCtrl := make(chan Event, 1)

	// Read loop. Makes sure that responses and events are processed.
	go func() {
		wg.Add(1)
		defer wg.Done()
		for {
			select {
			case <-quit:
				return
			default:
				resp, err := oled.ReadResponse()
				if err != nil {
					// Read error. Device is probably unreachable.
					sigs <- syscall.SIGHUP
					return
				} else if resp != nil {
					switch resp.(type) {
					case Event:
						switch resp.(Event).Screen {
						case Master:
							masterCtrl <- resp.(Event)
						case Slave:
							slaveCtrl <- resp.(Event)
						default:
							log.Printf("Got event 0x%02X for unknown screen 0x%02X.\n",
								resp.(Event).Event,
								resp.(Event).Screen)
						}
					}
				}
			}
		}
	}()

	// Start the handlers for the different screens, and specify which tag to show on them initially.
	go (&Screen{ID: Master, Controller: oled, Tag: 1, Events: masterCtrl, Quit: quit}).Run(&wg)
	go (&Screen{ID: Slave, Controller: oled, Tag: 2, Events: slaveCtrl, Quit: quit}).Run(&wg)

	// Wait for signal
	sig := <-sigs

	log.Println("Stopping due to", sig)
	signal.Reset() // Reset signal handling to terminate in case another one is issued

	close(quit)
	wg.Wait()

	if err := oled.Device.SetNonblocking(true); err == nil {
		// Consume any lingering messages.
		// This prevents junk from lying around in the HID pipe, causing failures next run.
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

	gArgs.temperatureUnit = flag.String("temperature-unit", "C", "Temperature unit to use (C/F/K)")

	gArgs.gmailCredentials = flag.String("gmail-credentials", "", "Path to JSON credential file for GMail access")
	gArgs.gmailLabel = flag.String("gmail-label", "INBOX", "For which label to count unread messages")

	gArgs.weatherKey = flag.String("weather-api-key", "", "API key to openweathermap.org")
	gArgs.weatherLocation = flag.String("weather-location", "", "The location to get the current weather as '<city>,<country>'")

	flag.Parse()

	for {
		for _, devInfo := range hid.Enumerate(VENDOR_ID, PRODUCT_ID) {
			found := false
			if runtime.GOOS != "linux" {
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
					oled := OLEDController{Device: device}
					oled.Run()
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
}
