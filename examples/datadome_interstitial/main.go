// Command datadome_interstitial solves a DataDome interstitial and reloads the page.
//
//	go run . -transport httpcloak -proxy http://user:pass@host:port
//	go run . -transport tlsclient -proxy http://user:pass@host:port
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/John-Avenue/john-antibot-sdk/datadome"
	"github.com/John-Avenue/john-antibot-sdk/examples/internal/exampleenv"
	"github.com/John-Avenue/john-antibot-sdk/flow"
)

func main() {
	env := exampleenv.Setup()
	defer env.Close()
	ctx := context.Background()

	page, err := env.Transport.Do(ctx, &flow.Request{Method: "GET", URL: env.PageURL, Headers: env.Browser.Navigate("")})
	if err != nil {
		log.Fatalf("FAIL page: %v", err)
	}
	dd := datadome.ParseDD(page.Body)
	if page.Status != 403 || dd.RT == "" {
		fmt.Printf("NO-CHALLENGE page returned %d without a DataDome block\n", page.Status)
		return
	}
	if dd.HardBlocked() {
		log.Fatalf("FAIL hard block (t=bv): rotate proxy/session")
	}
	if dd.RT != "i" {
		log.Fatalf("FAIL challenge type rt=%q t=%q is not the interstitial; use the slider example", dd.RT, dd.T)
	}

	res, err := flow.SolveInterstitial(ctx, env.Transport, env.Session, env.PageURL, dd, nil)
	if err != nil {
		log.Fatalf("FAIL interstitial: %v", err)
	}
	fmt.Println("interstitial view:", res.View, "userAgent:", env.Session.UserAgent)

	again, err := env.Transport.Do(ctx, &flow.Request{Method: "GET", URL: env.PageURL, Headers: env.Browser.Reload()})
	if err != nil {
		log.Fatalf("FAIL page re-GET: %v", err)
	}
	fmt.Println("PASS page re-GET", again.Status)
}
