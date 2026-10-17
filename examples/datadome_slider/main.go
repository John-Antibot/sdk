// Command datadome_slider solves a DataDome interstitial or slider until the page loads.
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

	for round := 0; round < 3; round++ {
		page, err := env.Transport.Do(ctx, &flow.Request{Method: "GET", URL: env.PageURL, Headers: env.Browser.Navigate("")})
		if err != nil {
			log.Fatalf("FAIL page: %v", err)
		}
		if page.Status == 200 || page.Status == 302 {
			fmt.Println("PASS page", page.Status, "userAgent:", env.Session.UserAgent)
			return
		}
		dd := datadome.ParseDD(page.Body)
		if dd.RT == "" {
			log.Fatalf("FAIL page returned %d without a DataDome block", page.Status)
		}
		if dd.HardBlocked() {
			log.Fatalf("FAIL hard block (t=bv): rotate proxy/session")
		}
		switch dd.RT {
		case "i":
			res, err := flow.SolveInterstitial(ctx, env.Transport, env.Session, env.PageURL, dd, nil)
			if err != nil {
				log.Fatalf("FAIL interstitial: %v", err)
			}
			fmt.Println("interstitial view:", res.View)
		case "c":
			res, err := flow.SolveSlider(ctx, env.Transport, env.Session, env.PageURL, dd, nil)
			if err != nil {
				if datadome.IsHardblock(err) {
					fmt.Println("HARDBLOCK captcha page served; rotate proxy/session")
					return
				}
				if datadome.IsPuzzleUnsupported(err) {
					fmt.Println("PUZZLE captcha page served; this generator solves the noPuzzle class only")
					return
				}
				log.Fatalf("FAIL slider: %v", err)
			}
			fmt.Println("slider solved, cookie stored:", res.Cookie.Name)
		default:
			log.Fatalf("FAIL unknown challenge rt=%q t=%q", dd.RT, dd.T)
		}
	}
	log.Fatalf("FAIL challenge loop after 3 rounds")
}
