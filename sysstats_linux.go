// Copyright 2020 Albert "Drauthius" Diserholt. All rights reserved.
// Licensed under the MIT License.

// +build linux

// Get system status from /proc/ and /sys/ (Linux edition)

package main

import (
	"log"
	"math"
	"time"

	linuxproc "github.com/c9s/goprocinfo/linux"
)

// Get system statistics at the specified interval.
// This will get the current CPU, memory, swap, and disk usage in fractions (0.0-1.0)
func SystemStats(interval time.Duration, results chan []float64, quit chan bool) {
	var prevIdle, prevTotal, prevIOTicks uint64
	var prevUptime float64

	defer close(results)

	for {
		var cpu, mem, swap, disk float64

		stats, err := linuxproc.ReadStat("/proc/stat")
		if err != nil {
			log.Println("Failed to retrieve stat information:", err)
		} else {
			idle := stats.CPUStatAll.Idle + stats.CPUStatAll.IOWait
			nonIdle := stats.CPUStatAll.User + stats.CPUStatAll.Nice + stats.CPUStatAll.System + stats.CPUStatAll.IRQ + stats.CPUStatAll.SoftIRQ + stats.CPUStatAll.Steal
			total := idle + nonIdle

			if prevIdle != 0 && prevTotal != 0 {
				totalDelta := total - prevTotal
				idleDelta := idle - prevIdle
				cpu = math.Max(float64(totalDelta-idleDelta)/float64(totalDelta), 0)
				if *gArgs.debug {
					log.Println("CPU%: ", cpu*100)
				}
			}

			prevIdle = idle
			prevTotal = total
		}

		meminfo, err := linuxproc.ReadMemInfo("/proc/meminfo")
		if err != nil {
			log.Println("Failed to retrieve meminfo information:", err)
		} else {
			mem = math.Max(float64(meminfo.MemTotal-meminfo.MemAvailable)/float64(meminfo.MemTotal), 0)
			swap = math.Max(float64(meminfo.SwapTotal-meminfo.SwapFree)/float64(meminfo.SwapTotal), 0)
			if math.IsInf(swap, 0) {
				// In case there is no swap.
				swap = 0
			}
			if *gArgs.debug {
				log.Println("Mem%: ", mem*100)
				log.Println("Swap%:", swap*100)
			}
		}

		uptime, err := linuxproc.ReadUptime("/proc/uptime")
		if err != nil {
			log.Println("Failed to retrieve uptime information:", err)
		} else {
			diskStats, err := linuxproc.ReadDiskStats("/proc/diskstats")
			if err != nil {
				log.Println("Failed to retrieve disk status information:", err)
			} else {
				for _, diskStat := range diskStats {
					if diskStat.Name == *gArgs.sysStatDisk {
						if prevIOTicks != 0 {
							disk = math.Max(float64(diskStat.IOTicks-prevIOTicks)/(uptime.Total-prevUptime)/1000, 0)
							if *gArgs.debug {
								log.Println("Disk%:", disk*100)
							}
						}
						prevIOTicks = diskStat.IOTicks
						break
					}
				}
			}

			prevUptime = uptime.Total
		}

		results <- []float64{cpu, mem, swap, disk}

		select {
		case <-quit:
			return
		case <-time.After(interval):
		}

	}
}
