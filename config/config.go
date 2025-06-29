package config

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Version variable, injected at compile time via -ldflags
var Version = "dev"

// Header represents a HTTP header key-value pair
type Header struct {
	Key, Value string
}

// HeadersList represents a list of HTTP headers
type HeadersList []Header

// IPArray represents a list of IP addresses
type IPArray []string

// Config holds all command line parameters
type Config struct {
	URL         string
	PostContent string
	Referer     string
	XForwardFor bool
	CustomIP    IPArray
	Headers     HeadersList
	Concurrent  int
}

// Flag methods
func (h *HeadersList) String() string {
	return fmt.Sprint(*h)
}

func (h *HeadersList) IsCumulative() bool {
	return true
}

func (h *HeadersList) Set(value string) error {
	res := strings.SplitN(value, ":", 2)
	if len(res) != 2 {
		return nil
	}
	*h = append(*h, Header{
		Key:   res[0],
		Value: strings.Trim(res[1], " "),
	})
	return nil
}

func (i *IPArray) String() string {
	return strings.Join(*i, ",")
}

func (i *IPArray) Set(value string) error {
	*i = append(*i, strings.TrimSpace(value))
	return nil
}

// PrintUsage prints usage information
func PrintUsage() {
	fmt.Fprintf(os.Stderr, `Usage: bandgo [-c concurrent] [-s target] [-p content] [-r refererUrl] [-f] [-i ip] [-H header]

Options:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Advanced Examples:
  bandgo -c 16 -s https://some.website -r https://referer.url -i 10.0.0.1 -i 10.0.0.2 
	16 concurrent to benchmark https://some.website with https://referer.url directly to IP 10.0.0.1 and 10.0.0.2

  bandgo -c 16 -s https://some.website -r https://referer.url
	16 concurrent to benchmark https://some.website with https://referer.url to DNS resolved IP address

  bandgo -s https://some.website -H "Authorization:Bearer token123" -H "Content-Type:application/json"
	Benchmark with custom headers
`)
}

// ParseArgs parses command line arguments
func ParseArgs() Config {
	var (
		showHelp    bool
		concurrent  int
		url         string
		postContent string
		referer     string
		xforwardfor bool
		customIP    IPArray
		headers     HeadersList
	)

	flag.BoolVar(&showHelp, "h", false, "show help")
	flag.IntVar(&concurrent, "c", 16, "concurrent threads for download, default 16")
	flag.StringVar(&url, "s", "", "target URL")
	flag.StringVar(&postContent, "p", "", "post content")
	flag.StringVar(&referer, "r", "", "referer URL")
	flag.BoolVar(&xforwardfor, "f", true, "randomize X-Forwarded-For and X-Real-IP address")
	flag.Var(&customIP, "i", "custom IP address for domain, multiple addresses will be assigned randomly")
	flag.Var(&headers, "H", "custom header in format 'Key:Value'")

	flag.Usage = PrintUsage
	flag.Parse()

	if showHelp {
		flag.Usage()
		os.Exit(0)
	}

	if url == "" {
		fmt.Fprintln(os.Stderr, "Error: -s parameter (target URL) is required!")
		flag.Usage()
		os.Exit(1)
	}

	return Config{
		URL:         url,
		PostContent: postContent,
		Referer:     referer,
		XForwardFor: xforwardfor,
		CustomIP:    customIP,
		Headers:     headers,
		Concurrent:  concurrent,
	}
}
