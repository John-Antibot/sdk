// Package exampleenv builds the examples' transport and session from flags.
package exampleenv

import (
	"flag"
	"log"
	"os"

	"github.com/John-Antibot/sdk/datadome"
	"github.com/John-Antibot/sdk/examples/transports/httpcloak"
	"github.com/John-Antibot/sdk/examples/transports/tlsclient"
	"github.com/John-Antibot/sdk/flow"
)

// Env is one example run's wiring.
type Env struct {
	Transport flow.Transport
	Browser   flow.Browser
	Session   *datadome.Session
	PageURL   string
	closeFn   func()
}

// Close releases the transport.
func (e *Env) Close() {
	if e.closeFn != nil {
		e.closeFn()
	}
}

// Setup parses flags and builds the transport and session.
func Setup() *Env {
	transport := flag.String("transport", "httpcloak", "TLS transport: httpcloak | tlsclient")
	proxy := flag.String("proxy", os.Getenv("DD_PROXY"), "proxy URL (or DD_PROXY)")
	pageURL := flag.String("url", "https://seatgeek.com/", "target page URL")
	gen := flag.String("gen", os.Getenv("DD_GEN"), "generator base URL (or DD_GEN)")
	flag.Parse()

	var t flow.Transport
	var closeFn func()
	switch *transport {
	case "httpcloak":
		hc := httpcloak.New(*proxy)
		t, closeFn = hc, hc.Close
	case "tlsclient":
		tc, err := tlsclient.New(*proxy)
		if err != nil {
			log.Fatalf("FAIL tls-client: %v", err)
		}
		t, closeFn = tc, tc.Close
	default:
		log.Fatalf("FAIL unknown -transport %q", *transport)
	}

	client, err := datadome.New(datadome.Config{Endpoint: *gen, APIKey: os.Getenv("JOHNANTIBOT_API_KEY")})
	if err != nil {
		log.Fatalf("FAIL client: %v", err)
	}
	browser := flow.DefaultBrowser
	session := client.Session(datadome.Session{
		UserAgent: browser.UserAgent,
		Referer:   *pageURL,
	})
	return &Env{Transport: t, Browser: browser, Session: session, PageURL: *pageURL, closeFn: closeFn}
}
