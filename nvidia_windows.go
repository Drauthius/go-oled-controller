// Copyright 2020 Albert "Drauthius" Diserholt. All rights reserved.
// Licensed under the MIT License.

// +build windows

// Get status of the first installed NVIDIA graphics card. Uses NVML to communicate with it.
// This Windows build uses the archived project from mxpv, because I couldn't get the official Go-bindings to
// cross compile on Linux.

package main

import (
	"log"
	"time"

	nvmlGo "github.com/mxpv/nvml-go"
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

	nvml, err := nvmlGo.New("")
	if err != nil {
		log.Println("Failed to load NVML library. Is it not installed?", err)
		return
	}

	nvml.Init()
	defer nvml.Shutdown()

	count, err := nvml.DeviceGetCount()
	if err != nil {
		log.Println("Failed to get device count:", err)
		return
	} else if count < 1 {
		log.Println("Found no NVIDIA device.")
		return
	}

	device, err := nvml.DeviceGetHandleByIndex(0)
	if err != nil {
		log.Println("Failed to create device:", err)
		return
	}

	handleError := func(s string, err error) {
		if err != nil {
			log.Println("Failed to get "+s+" from NVIDIA card:", err)
		}
	}

	m := map[uint32]uint32{
		1: 250,
		2: 500,
		3: 985,
		4: 1969,
	}

	for {
		temp, err := nvml.DeviceGetTemperature(device, nvmlGo.TemperatureGPU)
		handleError("temperature", err)
		var temperature float64
		switch unit {
		case "F":
			temperature = float64(temp)*9/5 + 32
		case "K":
			temperature = float64(temp) + 273.15
		default:
			fallthrough
		case "C":
			temperature = float64(temp)
		}

		fanSpeed, err := nvml.DeviceGetFanSpeed(device)
		handleError("fan speed", err)

		utilization, err := nvml.DeviceGetUtilizationRates(device)
		handleError("utilization", err)

		encoder, _, err := nvml.DeviceGetEncoderUtilization(device)
		handleError("encoder", err)

		decoder, _, err := nvml.DeviceGetDecoderUtilization(device)
		handleError("decoder", err)

		rx, err := nvml.DeviceGetPCIeThroughput(device, nvmlGo.PCIeUtilRXBytes)
		handleError("PCIe RX", err)
		tx, err := nvml.DeviceGetPCIeThroughput(device, nvmlGo.PCIeUtilRXBytes)
		handleError("PCIe TX", err)

		gen, err := nvml.DeviceGetMaxPcieLinkGeneration(device)
		handleError("PCIe Gen", err)
		width, err := nvml.DeviceGetMaxPcieLinkWidth(device)
		handleError("PCIe link width", err)

		bandwidth := m[gen] * width

		results <- GraphicCardResult{
			Temperature:  temperature,
			FanSpeed:     float64(fanSpeed) / 100,
			GPU:          float64(utilization.GPU) / 100,
			Memory:       float64(utilization.Memory) / 100,
			Encoder:      float64(encoder) / 100,
			Decoder:      float64(decoder) / 100,
			PCIBandwidth: float64(rx+tx) / float64(bandwidth),
		}

		select {
		case <-time.After(interval):
		case <-quit:
			return
		}
	}
}
