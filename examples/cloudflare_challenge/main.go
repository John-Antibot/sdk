// Command cloudflare_challenge clears a Cloudflare challenge page through a proxy, then loads the page
// from your own connection through the same proxy with the clearance.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/John-Avenue/john-antibot-sdk/cloudflare"
	"github.com/John-Avenue/john-antibot-sdk/examples/transports/httpcloak"
	"github.com/John-Avenue/john-antibot-sdk/flow"
)

func main() {
	pageURL := flag.String("url", "https://www.example.com/", "page that serves the challenge (approved for your key)")
	proxy := flag.String("proxy", os.Getenv("CF_PROXY"), "proxy URL, a sticky session or a static proxy (or CF_PROXY)")
	timezone := flag.String("timezone", "America/New_York", "IANA time zone of the proxy's location")
	api := flag.String("api", os.Getenv("CF_API"), "API base URL (or CF_API), default the hosted API")
	flag.Parse()
	if *proxy == "" {
		log.Fatal("FAIL -proxy (or CF_PROXY) is required")
	}

	cf, err := cloudflare.New(cloudflare.Config{APIKey: os.Getenv("JOHNANTIBOT_API_KEY"), Endpoint: *api})
	if err != nil {
		log.Fatalf("FAIL client: %v", err)
	}
	t := httpcloak.New(*proxy)
	defer t.Close()
	ctx := context.Background()

	get := func(browser flow.Browser) *flow.Response {
		res, err := t.Do(ctx, &flow.Request{Method: "GET", URL: *pageURL, Headers: browser.Navigate("")})
		if err != nil {
			log.Fatalf("FAIL page: %v", err)
		}
		return res
	}
	fmt.Println("page", get(flow.DefaultBrowser).Status)

	// A declined job is the site turning this attempt down: send another.
	var res *cloudflare.Result
	for attempt := 1; ; attempt++ {
		solveCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		res, err = cf.SolveChallenge(solveCtx, &cloudflare.ChallengeInput{URL: *pageURL, Proxy: *proxy, Timezone: *timezone})
		cancel()
		if err == nil || !cloudflare.IsDeclined(err) || attempt == 3 {
			break
		}
	}
	if err != nil {
		log.Fatalf("FAIL solve: %v", err)
	}
	fmt.Printf("solved in %d ms from %s\n", res.Ms, res.ExitIP)

	// Spend the clearance like the browser it is for: its cookies and its user agent, from the same proxy.
	u, err := url.Parse(*pageURL)
	if err != nil {
		log.Fatal(err)
	}
	for _, c := range res.Cookies {
		domain := c.Domain
		if domain == "" {
			domain = u.Hostname()
		}
		t.SetCookie(domain, c.Name, c.Value)
	}
	browser := flow.DefaultBrowser
	browser.UserAgent = res.UserAgent
	status := get(browser).Status
	fmt.Println("page with clearance", status)
	if status == 403 {
		log.Fatal("FAIL the site still challenges: does the proxy keep its address?")
	}
	fmt.Println("PASS userAgent:", res.UserAgent)
}
