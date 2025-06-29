package main

import (
	"sync"

	"bandgo/config"
	"bandgo/monitor"
	"bandgo/worker"
)

// Version variable, injected at compile time via -ldflags
var Version = "dev"

func main() {
	// Print banner and version
	println("BandGo - Make your bandwidth GO away! -", Version)

	// Parse command line arguments
	cfg := config.ParseArgs()

	// Determine number of workers
	workers := cfg.Concurrent
	if len(cfg.CustomIP) > 0 && workers < len(cfg.CustomIP) {
		workers = len(cfg.CustomIP)
	}

	if workers <= 0 {
		workers = 16
	}

	// Start network traffic monitor
	go monitor.MonitorNetworkTraffic(cfg.URL, workers)

	// Start workers
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go worker.StartWorker(&wg, cfg)
	}

	wg.Wait()
	monitor.Reset()
}
