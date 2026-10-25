// Command datadome_tags loads a page and posts the ch and le tags payloads.
package main

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"github.com/John-Antibot/sdk/datadome"
	"github.com/John-Antibot/sdk/examples/internal/exampleenv"
	"github.com/John-Antibot/sdk/flow"
)

func main() {
	env := exampleenv.Setup()
	defer env.Close()
	ctx := context.Background()

	page, err := env.Transport.Do(ctx, &flow.Request{Method: "GET", URL: env.PageURL, Headers: env.Browser.Navigate("")})
	if err != nil {
		log.Fatalf("FAIL page: %v", err)
	}
	fmt.Println("page", page.Status)

	u, err := url.Parse(env.PageURL)
	if err != nil {
		log.Fatal(err)
	}
	jsURL := "https://datadome." + u.Hostname() + "/js/"
	siteKey := datadome.ParseDD(page.Body).Hash

	for _, event := range []string{datadome.EventCH, datadome.EventLE} {
		cookie := env.Transport.Cookie(env.PageURL, "datadome")
		status, err := flow.SendTags(ctx, env.Transport, env.Session, jsURL, &datadome.TagsInput{
			Event:   event,
			Cookie:  cookie,
			SiteKey: siteKey,
			Referer: env.PageURL,
		}, nil)
		if err != nil {
			log.Fatalf("FAIL tags %s: %v", event, err)
		}
		fmt.Println("tags", event, "->", status)
	}
	fmt.Println("PASS userAgent:", env.Session.UserAgent)
}
