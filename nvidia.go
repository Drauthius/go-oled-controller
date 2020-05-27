// Copyright 2020 Albert "Drauthius" Diserholt. All rights reserved.
// Licensed under the MIT License.

// Get status of the first installed NVIDIA graphics card. Uses NVML to communicate with it.

package main

import (
	"log"
	"time"

	"gitlab.com/Drauthius/gpu-monitoring-tools/bindings/go/nvml"
)

// The type of a graphic card result
type GraphicCardResult struct {
	Temperature  float64 // The temperature in the desired unit.
	FanSpeed     float64 // The intended fan speed in percent (0-1).
	GPU          float64 // The GPU utilization in percent (0-1).
	Memory       float64 // The memory utilization in percent (0-1).
	Encoder      float64 // The encoder utilization in percent (0-1).
	Decoder      float64 // The decoder utilization in percent (0-1).
	PCIBandwidth float64 // The PCIe bandwidth utilization in percent (0-1).
}

// Run a loop that will continuously get status from the NVIDIA graphics card, at the specified interval.
func GraphicCardStats(interval time.Duration, unit string, results chan GraphicCardResult, quit chan bool) {
	defer close(results)

	if err := nvml.Init(); err != nil {
		log.Println("Failed to initiate NVML:", err)
		return
	}
	defer nvml.Shutdown()

	count, err := nvml.GetDeviceCount()
	if err != nil {
		log.Println("Failed to get device count:", err)
		return
	} else if count < 1 {
		log.Println("Found no NVIDIA device.")
		return
	}

	device, err := nvml.NewDevice(0)
	if err != nil {
		log.Println("Failed to create device:", err)
		return
	}

	for {
		status, err := device.Status()
		if err != nil {
			log.Println("Failed to get device status:", err)
			return
		}

		var temperature float64
		switch unit {
		case "F":
			temperature = float64(*status.Temperature)*9/5 + 32
		case "K":
			temperature = float64(*status.Temperature) + 273.15
		default:
			fallthrough
		case "C":
			temperature = float64(*status.Temperature)
		}
		results <- GraphicCardResult{
			Temperature:  temperature,
			FanSpeed:     float64(*status.FanSpeed) / 100,
			GPU:          float64(*status.Utilization.GPU) / 100,
			Memory:       float64(*status.Utilization.Memory) / 100,
			Encoder:      float64(*status.Utilization.Encoder) / 100,
			Decoder:      float64(*status.Utilization.Decoder) / 100,
			PCIBandwidth: float64(*status.PCI.Throughput.RX+*status.PCI.Throughput.TX) / float64(*device.PCI.Bandwidth),
		}

		select {
		case <-time.After(interval):
		case <-quit:
			return
		}
	}
}
