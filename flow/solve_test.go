package flow

import (
	"context"
	"testing"
)

type cookieRecorder struct{ domain, name, value string }

func (c *cookieRecorder) Do(context.Context, *Request) (*Response, error) { return nil, nil }
func (c *cookieRecorder) SetCookie(domain, name, value string) {
	c.domain, c.name, c.value = domain, name, value
}
func (c *cookieRecorder) Cookie(string, string) string { return "" }

func TestReplyCookieWithoutDomainBelongsToThePage(t *testing.T) {
	var c cookieRecorder
	if _, err := storeReplyCookie(&c, geoReply{Cookie: "datadome=v1; Max-Age=604800; Path=/; Secure; SameSite=Lax"}, "https://seatgeek.com/x-tickets/1"); err != nil {
		t.Fatal(err)
	}
	if c.domain != ".seatgeek.com" || c.value != "v1" {
		t.Fatalf("stored %+v; want .seatgeek.com", c)
	}
	if _, err := storeReplyCookie(&c, geoReply{Cookie: "datadome=v2; Domain=.example.com; Path=/"}, "https://seatgeek.com/"); err != nil || c.domain != ".example.com" {
		t.Fatalf("reply domain not kept: %+v %v", c, err)
	}
}
