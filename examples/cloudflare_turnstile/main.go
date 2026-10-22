// Command cloudflare_turnstile solves a Turnstile widget through a proxy and prints the token.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/John-Avenue/john-antibot-sdk/cloudflare"
)

func main() {
	pageURL := flag.String("url", "https://www.example.com/login", "page with the widget (approved for your key)")
	siteKey := flag.String("sitekey", "", "site key of the widget")
	proxy := flag.String("proxy", os.Getenv("CF_PROXY"), "proxy URL, a sticky session or a static proxy (or CF_PROXY)")
	timezone := flag.String("timezone", "America/New_York", "IANA time zone of the proxy's location")
	api := flag.String("api", os.Getenv("CF_API"), "API base URL (or CF_API), default the hosted API")
	flag.Parse()
	if *proxy == "" || *siteKey == "" {
		log.Fatal("FAIL -proxy (or CF_PROXY) and -sitekey are required")
	}

	cf, err := cloudflare.New(cloudflare.Config{APIKey: os.Getenv("JOHNANTIBOT_API_KEY"), Endpoint: *api})
	if err != nil {
		log.Fatalf("FAIL client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := cf.SolveTurnstile(ctx, &cloudflare.TurnstileInput{PageURL: *pageURL, SiteKey: *siteKey, Proxy: *proxy, Timezone: *timezone})
	if err != nil {
		log.Fatalf("FAIL solve: %v", err)
	}
	// Send the token as cf-turnstile-response with the site's form, from the same proxy and the user agent in the result.
	fmt.Printf("PASS token %d bytes in %d ms from %s\nuserAgent: %s\n", len(res.Token()), res.Ms, res.ExitIP, res.UserAgent)
}
