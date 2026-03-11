package main

import (
	"log"
	"os"
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
	var agg *monitor.Aggregator
	if !cfg.NoTUI {
		agg = monitor.NewAggregator()
		go func() {
			if err := monitor.StartTUI(cfg.URL, workers, agg); err != nil {
				log.Printf("tui exited with error: %v", err)
			}
			os.Exit(0)
		}()
	}

	// Start workers
	var wg sync.WaitGroup
	for i := 1; i <= workers; i++ {
		wg.Add(1)
		go worker.StartWorker(&wg, i, cfg, agg)
	}

	wg.Wait()
}
