package monitor

import (
	"fmt"
	"os"
	"time"

	"github.com/apoorvam/goterminal"
	netstat "github.com/shirou/gopsutil/net"

	"bandgo/utils"
)

var terminalWriter = goterminal.New(os.Stdout)

// MonitorNetworkTraffic monitors and displays network traffic statistics
func MonitorNetworkTraffic(url string, concurrent int) {
	initialNetCounter, _ := netstat.IOCounters(true)
	// Target information
	fmt.Println("Traffic Monitor")
	fmt.Printf("Target: %s\n", url)

	// Traffic totals
	var totalRecv float64 = 0
	var totalSent float64 = 0
	startTime := time.Now()

	// Update interval
	ticker := time.NewTicker(1 * time.Second)

	for {
		<-ticker.C
		netCounter, _ := netstat.IOCounters(true)

		// Clear previous output
		terminalWriter.Clear()

		// Print elapsed time
		elapsed := time.Since(startTime)
		fmt.Fprintf(terminalWriter, "Elapsed: %s\n\n", elapsed.Round(time.Second))

		// Print current traffic rates
		fmt.Fprintln(terminalWriter, "Current Traffic Rates:")
		fmt.Fprintf(terminalWriter, "%-12s %-15s %-15s\n", "Interface", "Download", "Upload")
		fmt.Fprintln(terminalWriter, "----------------------------------------")

		// Process network interfaces
		for i := range netCounter {
			if netCounter[i].BytesRecv == 0 && netCounter[i].BytesSent == 0 {
				continue
			}
			recvBytes := float64(netCounter[i].BytesRecv - initialNetCounter[i].BytesRecv)
			sendBytes := float64(netCounter[i].BytesSent - initialNetCounter[i].BytesSent)

			totalRecv += recvBytes
			totalSent += sendBytes

			fmt.Fprintf(terminalWriter, "%-12s %-15s %-15s\n",
				netCounter[i].Name,
				utils.ReadableBytes(recvBytes)+"/s",
				utils.ReadableBytes(sendBytes)+"/s")
		}

		// Print totals
		fmt.Fprintf(terminalWriter, "\nTraffic Totals:\n")
		fmt.Fprintf(terminalWriter, "Total Downloaded: %s\n", utils.ReadableBytes(totalRecv))
		fmt.Fprintf(terminalWriter, "Total Uploaded:   %s\n", utils.ReadableBytes(totalSent))

		// Print workers info
		fmt.Fprintf(terminalWriter, "\nConcurrent Workers: %d\n", concurrent)

		// Update terminal
		terminalWriter.Print()

		// Update counters for next iteration
		initialNetCounter = netCounter
	}
}

// Reset resets the terminal writer
func Reset() {
	terminalWriter.Reset()
}
