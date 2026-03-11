package worker

import (
	"context"
	"crypto/tls"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"bandgo/config"
	"bandgo/monitor"
	"bandgo/utils"

	rand_ua "github.com/xrgzs/rand-ua-go"
)

// processHeaders processes and sets headers for the HTTP request
func processHeaders(req *http.Request, cfg config.Config) {
	// Set basic headers
	ua := rand_ua.MustGetRandomUA()
	// log.Printf("Using User-Agent: %s\n", ua)
	req.Header.Add("User-Agent", ua)

	// Set referer if provided
	if cfg.Referer != "" {
		req.Header.Add("Referer", cfg.Referer)
	} else {
		req.Header.Add("Referer", cfg.URL)
	}

	// Add X-Forwarded-For if enabled
	if cfg.XForwardFor {
		randomIP := utils.GenerateRandomIPAddress()
		req.Header.Add("X-Forwarded-For", randomIP)
		req.Header.Add("X-Real-IP", randomIP)
	}
	// Add custom headers
	for _, head := range cfg.Headers {
		headKey := head.Key
		headValue := head.Value

		// Handle Random header keys
		if strings.HasPrefix(head.Key, "Random") {
			count, convErr := strconv.Atoi(strings.ReplaceAll(head.Key, "Random", ""))
			if convErr == nil {
				headKey = utils.RandStringBytesMaskImpr(count)
			}
		}

		// Handle Random header values
		if strings.HasPrefix(head.Value, "Random") {
			count, convErr := strconv.Atoi(strings.ReplaceAll(head.Value, "Random", ""))
			if convErr == nil {
				headValue = utils.RandStringBytesMaskImpr(count)
			}
		}

		req.Header.Del(headKey)
		req.Header.Set(headKey, headValue)
	}
}

// createTransport creates an HTTP transport with custom IP support
func createTransport(customIP config.IPArray) *http.Transport {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// If custom IPs are provided, configure dialers
	if len(customIP) > 0 {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}

		dialFunc := func(ctx context.Context, network, addr string) (net.Conn, error) {
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			ip := customIP[r.Intn(len(customIP))]

			hostPort := ":80"
			if strings.HasPrefix(addr, "https") {
				hostPort = ":443"
			}

			return dialer.DialContext(ctx, network, ip+hostPort)
		}

		transport.DialContext = dialFunc
		transport.DialTLSContext = dialFunc
	}

	return transport
}

// StartWorker starts a worker that performs HTTP requests
func StartWorker(wg *sync.WaitGroup, workerID int, cfg config.Config, agg *monitor.Aggregator) {
	defer wg.Done()
	defer func() {
		if r := recover(); r != nil {
			// Restart worker on panic
			wg.Add(1)
			go StartWorker(wg, workerID, cfg, agg)
		}
	}()

	if agg != nil {
		agg.RegisterWorker(workerID)
	}

	transport := createTransport(cfg.CustomIP)
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// Loop
	for {
		var req *http.Request
		var err error

		// Create request based on method
		if cfg.PostContent != "" {
			req, err = http.NewRequest("POST", cfg.URL, strings.NewReader(cfg.PostContent))
		} else {
			req, err = http.NewRequest("GET", cfg.URL, nil)
		}

		if err != nil {
			if agg != nil {
				agg.ResetWorker(workerID)
			}
			continue
		}

		// Setup request headers
		processHeaders(req, cfg)

		// Execute request
		resp, err := client.Do(req)
		if err != nil {
			if agg != nil {
				agg.ResetWorker(workerID)
			}
			continue
		}

		if agg != nil {
			agg.SetContentLength(workerID, resp.ContentLength)
		}

		if agg == nil {
			// Fast path for no-TUI mode: no aggregation, discard directly.
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			continue
		}

		buf := make([]byte, 32*1024)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				agg.AddDownloaded(workerID, n)
			}
			if readErr != nil {
				if readErr == io.EOF {
					break
				}
				break
			}
		}
		resp.Body.Close()
		agg.ResetWorker(workerID)
	}
}
