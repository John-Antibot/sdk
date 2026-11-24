// Command kasada_payload makes a Kasada /tl body from an ips.js your
// client already fetched and prints where to post it. Posting it with
// that same client and proxy is left to the caller, as in production.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/John-Antibot/sdk/kasada"
)

func main() {
	scriptFile := flag.String("script", "", "the ips.js your client fetched")
	htmlFile := flag.String("html", "", "the block or /fp page that named ips.js (or use -ips-link)")
	ipsLink := flag.String("ips-link", "", "the ips.js path and query from that page (or use -html)")
	pageURL := flag.String("url", "https://www.example.com/", "the block or /fp page URL")
	userAgent := flag.String("user-agent", os.Getenv("KASADA_USER_AGENT"), "your client's user agent: Chrome 156, 157 or 158 on macOS (or KASADA_USER_AGENT); optional, the service picks one and the result names it")
	ip := flag.String("ip", os.Getenv("KASADA_EXIT_IP"), "the IP the /tl request will be posted from (or KASADA_EXIT_IP)")
	language := flag.String("accept-language", "en-US,en;q=0.9", "your client's Accept-Language")
	api := flag.String("api", os.Getenv("KASADA_API"), "API base URL (or KASADA_API), default the hosted API")
	flag.Parse()
	if *scriptFile == "" || (*htmlFile == "" && *ipsLink == "") || *ip == "" {
		log.Fatal("FAIL -script, one of -html/-ips-link and -ip are required")
	}
	script, err := os.ReadFile(*scriptFile)
	if err != nil {
		log.Fatalf("FAIL read script: %v", err)
	}
	var html []byte
	if *htmlFile != "" {
		if html, err = os.ReadFile(*htmlFile); err != nil {
			log.Fatalf("FAIL read html: %v", err)
		}
	}
	client, err := kasada.New(kasada.Config{APIKey: os.Getenv("JOHNANTIBOT_API_KEY"), Endpoint: *api})
	if err != nil {
		log.Fatalf("FAIL client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	res, err := client.GeneratePayload(ctx, &kasada.PayloadInput{
		Script: string(script), HTML: string(html), IpsLink: *ipsLink, PageURL: *pageURL,
		UserAgent: *userAgent, IP: *ip, AcceptLanguage: *language,
	})
	if err != nil {
		log.Fatalf("FAIL payload: %v", err)
	}
	fmt.Printf("PASS %d payload bytes in %s; POST them unchanged to %s as %q\n", len(res.Payload), time.Since(start).Round(100*time.Millisecond), res.TlURL, res.UserAgent)
	names := make([]string, 0, len(res.Headers))
	for name := range res.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("%s: %s\n", name, res.Headers[name])
	}
}
