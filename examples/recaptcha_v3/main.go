// Command recaptcha_v3 makes a reCAPTCHA v3 token for a page through a proxy and prints it with the browser it was made as.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/John-Antibot/sdk/recaptcha"
)

func main() {
	pageURL := flag.String("url", "https://www.example.com/login", "page that asks for the token (approved for your key)")
	siteKey := flag.String("sitekey", "", "the site's reCAPTCHA key")
	action := flag.String("action", "", "the action the page passes to execute (default submit)")
	enterprise := flag.Bool("enterprise", false, "the page loads reCAPTCHA Enterprise (enterprise.js), not the classic script")
	proxy := flag.String("proxy", os.Getenv("RECAPTCHA_PROXY"), "proxy URL, a public address (or RECAPTCHA_PROXY)")
	api := flag.String("api", os.Getenv("RECAPTCHA_API"), "API base URL (or RECAPTCHA_API), default the hosted API")
	flag.Parse()
	if *proxy == "" || *siteKey == "" {
		log.Fatal("FAIL -proxy (or RECAPTCHA_PROXY) and -sitekey are required")
	}

	rc, err := recaptcha.New(recaptcha.Config{APIKey: os.Getenv("JOHNANTIBOT_API_KEY"), Endpoint: *api})
	if err != nil {
		log.Fatalf("FAIL client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	in := &recaptcha.Input{URL: *pageURL, SiteKey: *siteKey, Action: *action, Proxy: *proxy}
	solve := rc.SolveV3
	if *enterprise {
		solve = rc.SolveEnterprise
	}
	start := time.Now()
	res, err := solve(ctx, in)
	if err != nil {
		log.Fatalf("FAIL solve: %v", err)
	}
	// Send the token to the site's own request (the form field g-recaptcha-response, or whatever the page does with it), from the same
	// proxy, with the headers of the browser it was made as. Google stops accepting it at ExpiresAt.
	fmt.Printf("PASS token %d bytes in %s, valid until %s\n", len(res.Token), time.Since(start).Round(100*time.Millisecond), res.ExpiresAt.Format(time.RFC3339))
	headers := res.Headers()
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("%s: %s\n", name, headers.Get(name))
	}
}
