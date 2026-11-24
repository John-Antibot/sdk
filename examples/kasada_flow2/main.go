// Command kasada_flow2 shows what a Flow 2 site (p.js, then /mfc, /fp, /tl) needs from the API, and what it does not.
//
// Two calls reach the API, each once per session and neither per request:
//
//	Version  the page's p.js  -> x-kpsdk-v, and what x-kpsdk-cd is made from (free; answered from memory for a known p.js)
//	Payload  the /fp page's ips.js -> the /tl body and headers (see kasada_payload)
//
// Every protected request after /tl then carries x-kpsdk-cd, which the SDK computes on your side: no call, no charge.
// This command stands in for that last step: give it the p.js you fetched and the ct and st that /tl answered with.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/John-Antibot/sdk/kasada"
)

func main() {
	pjsFile := flag.String("pjs", "", "the p.js your client fetched")
	pageURL := flag.String("url", "https://www.example.com/", "the page that loaded p.js")
	ct := flag.String("ct", "", "the x-kpsdk-ct header /tl answered with (or the latest reply's)")
	st := flag.String("st", "", "the x-kpsdk-st header /tl answered with, in milliseconds")
	api := flag.String("api", os.Getenv("KASADA_API"), "API base URL (or KASADA_API), default the hosted API")
	flag.Parse()
	stMs, err := strconv.ParseInt(*st, 10, 64)
	if *pjsFile == "" || *ct == "" || err != nil {
		log.Fatal("FAIL -pjs, -ct and a numeric -st are required")
	}
	pjs, err := os.ReadFile(*pjsFile)
	if err != nil {
		log.Fatalf("FAIL read p.js: %v", err)
	}
	client, err := kasada.New(kasada.Config{APIKey: os.Getenv("JOHNANTIBOT_API_KEY"), Endpoint: *api})
	if err != nil {
		log.Fatalf("FAIL client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	version, err := client.Version(ctx, &kasada.VersionInput{Script: string(pjs), PageURL: *pageURL})
	if err != nil {
		log.Fatalf("FAIL version: %v", err)
	}
	fmt.Printf("x-kpsdk-v: %s (send it on /fp and on every protected request)\n", version.Version)

	// For each protected request, from here on, with no further call:
	start := time.Now()
	cd, err := version.CDHeader(*ct, stMs)
	if err != nil {
		log.Fatalf("FAIL cd: %v", err)
	}
	fmt.Printf("x-kpsdk-cd (made locally in %s): %s\n", time.Since(start).Round(time.Microsecond), cd)
}
