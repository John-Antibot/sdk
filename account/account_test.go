package account

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func serve(t *testing.T, status int, body string) (*Client, *http.Request) {
	t.Helper()
	var seen http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = *r.Clone(r.Context())
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	c, err := New(Config{Endpoint: srv.URL + "/", APIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	return c, &seen
}

func TestBalanceReadsTheAccountOfTheKey(t *testing.T) {
	c, seen := serve(t, 200, `{"metered":true,"balanceMicros":4821000}`)
	b, err := c.Balance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !b.Metered || b.Micros != 4821000 || b.Dollars() != 4.821 {
		t.Errorf("balance = %+v (%v dollars)", b, b.Dollars())
	}
	// a GET of /balance under the endpoint, with the key in the header the API reads
	if seen.Method != http.MethodGet || seen.URL.Path != "/balance" || seen.Header.Get("x-api-key") != "key" {
		t.Errorf("sent %s %s with key %q", seen.Method, seen.URL.Path, seen.Header.Get("x-api-key"))
	}
}

func TestAKeyThatIsNotMeteredHasNoBalance(t *testing.T) {
	c, _ := serve(t, 200, `{"metered":false}`)
	b, err := c.Balance(context.Background())
	if err != nil || b.Metered || b.Micros != 0 {
		t.Errorf("%+v %v", b, err)
	}
}

func TestRefusalsAreTypedAndKeepTheAPIsText(t *testing.T) {
	c, _ := serve(t, 401, `{"error":"gateway: missing or bad x-api-key"}`)
	_, err := c.Balance(context.Background())
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != 401 || api.Message != "gateway: missing or bad x-api-key" || !IsBadKey(err) {
		t.Errorf("%v", err)
	}
	// other failures are not a bad key
	c, _ = serve(t, 503, `We cannot load the billing data.`)
	if _, err := c.Balance(context.Background()); IsBadKey(err) || err == nil {
		t.Errorf("a 503 is no bad key: %v", err)
	}
}

func TestADefaultEndpointAndATimeout(t *testing.T) {
	c, _ := New(Config{})
	if c.Endpoint() != DefaultEndpoint {
		t.Errorf("endpoint = %q", c.Endpoint())
	}
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer hang.Close()
	slow, _ := New(Config{Endpoint: hang.URL, Timeout: 100 * time.Millisecond})
	start := time.Now()
	if _, err := slow.Balance(context.Background()); err == nil || time.Since(start) > 3*time.Second {
		t.Errorf("a call that never answers ends with its timeout: %v after %s", err, time.Since(start))
	}
}
