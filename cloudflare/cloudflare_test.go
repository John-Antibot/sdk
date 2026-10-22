package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const doneJob = `{"id":"job_1","state":"done","task":"challenge","result":{"payload":"cf_clearance=abc123","userAgent":"UA/1","session":"sess_x","exitIp":"9.9.9.9","ms":7000,` +
	`"cookies":[{"name":"cf_clearance","value":"abc123","domain":"example.com","path":"/"},{"name":"__cf_bm","value":"bm","domain":"example.com","path":"/"}]}}`

// api is a stand-in for the API: submit answers 202 with job_1, the first poll says running, the next says terminal.
type api struct {
	t         *testing.T
	terminal  string
	submitted map[string]any
	polls     atomic.Int32
	waits     []string
}

func (a *api) handler() http.Handler {
	mux := http.NewServeMux()
	submit := func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "key" || r.Header.Get("x-org-key") != "org" {
			a.t.Errorf("headers: key %q org %q", r.Header.Get("x-api-key"), r.Header.Get("x-org-key"))
		}
		b, _ := io.ReadAll(r.Body)
		a.submitted = map[string]any{}
		_ = json.Unmarshal(b, &a.submitted)
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"id":"job_1","state":"queued"}`)
	}
	mux.HandleFunc("POST /challenge", submit)
	mux.HandleFunc("POST /turnstile", submit)
	mux.HandleFunc("GET /jobs/job_1", func(w http.ResponseWriter, r *http.Request) {
		a.waits = append(a.waits, r.URL.Query().Get("wait"))
		if a.polls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"id":"job_1","state":"running"}`)
			return
		}
		_, _ = io.WriteString(w, a.terminal)
	})
	return mux
}

func newClient(t *testing.T, a *api) *Client {
	t.Helper()
	srv := httptest.NewServer(a.handler())
	t.Cleanup(srv.Close)
	c, err := New(Config{Endpoint: srv.URL + "/", APIKey: "key", OrgKey: "org"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSolveChallengeSubmitsAndWaits(t *testing.T) {
	a := &api{t: t, terminal: doneJob}
	c := newClient(t, a)
	res, err := c.SolveChallenge(context.Background(), &ChallengeInput{
		URL: "https://example.com/", Proxy: "u:p@8.8.8.8:80", Timezone: "America/New_York", Timeout: 90 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Clearance() != "abc123" || res.UserAgent != "UA/1" || res.ExitIP != "9.9.9.9" || res.Session != "sess_x" || len(res.Cookies) != 2 || res.Ms != 7000 {
		t.Errorf("result = %+v", res)
	}
	// what was sent: the fields given, none of the ones left empty
	want := map[string]any{"url": "https://example.com/", "proxy": "u:p@8.8.8.8:80", "timezone": "America/New_York", "timeoutMs": float64(90000)}
	if len(a.submitted) != len(want) {
		t.Errorf("submitted %v", a.submitted)
	}
	for k, v := range want {
		if a.submitted[k] != v {
			t.Errorf("submitted %s = %v, want %v", k, a.submitted[k], v)
		}
	}
	// it polled with the server's wait, twice (running, then done)
	if a.polls.Load() != 2 || a.waits[0] != "25" {
		t.Errorf("polls %d, waits %v", a.polls.Load(), a.waits)
	}
}

func TestSolveTurnstile(t *testing.T) {
	a := &api{t: t, terminal: `{"id":"job_1","state":"done","task":"turnstile","result":{"payload":"0.token","userAgent":"UA/1"}}`}
	c := newClient(t, a)
	res, err := c.SolveTurnstile(context.Background(), &TurnstileInput{PageURL: "https://example.com/login", SiteKey: "0x4AAA", Proxy: "8.8.8.8:80", Action: "login"})
	if err != nil || res.Token() != "0.token" || res.Clearance() != "" {
		t.Fatalf("%+v %v", res, err)
	}
	if a.submitted["pageUrl"] != "https://example.com/login" || a.submitted["sitekey"] != "0x4AAA" || a.submitted["action"] != "login" || a.submitted["cData"] != nil {
		t.Errorf("submitted %v", a.submitted)
	}
}

func TestEndedJobsAreErrors(t *testing.T) {
	for _, tc := range []struct {
		name, terminal string
		check          func(error) bool
		retry          bool
	}{
		{"declined", `{"id":"job_1","state":"declined","error":"the challenge was not passed","retry":true}`, IsDeclined, true},
		{"failed", `{"id":"job_1","state":"failed","error":"the proxy is refused: x is not a public address"}`, func(err error) bool { return !IsDeclined(err) }, false},
		{"no wall", `{"id":"job_1","state":"failed","error":"the page serves no challenge (status 200)"}`, IsNoChallenge, false},
		{"rotating proxy", `{"id":"job_1","state":"failed","error":"the proxy exits from a different address on each connection (1.1.1.1, then 2.2.2.2): a clearance is for one address"}`, IsRotatingProxy, false},
	} {
		a := &api{t: t, terminal: tc.terminal}
		_, err := newClient(t, a).SolveChallenge(context.Background(), &ChallengeInput{URL: "https://example.com/", Proxy: "8.8.8.8:80"})
		var job *JobError
		if !errors.As(err, &job) || !tc.check(err) || job.Retry != tc.retry || job.ID != "job_1" || job.Message == "" {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

func TestRefusals(t *testing.T) {
	for _, tc := range []struct {
		status int
		check  func(error) bool
	}{{402, IsInsufficientBalance}, {403, IsSiteNotApproved}, {429, IsRateLimited}, {400, IsBadRequest}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, `{"error":"gateway: nope"}`)
		}))
		c, _ := New(Config{Endpoint: srv.URL})
		_, err := c.SolveChallenge(context.Background(), &ChallengeInput{URL: "https://example.com/", Proxy: "8.8.8.8:80"})
		srv.Close()
		var api *APIError
		if !errors.As(err, &api) || api.StatusCode != tc.status || api.Message != "gateway: nope" || !tc.check(err) {
			t.Errorf("%d: %v", tc.status, err)
		}
	}
	// a refusal is not a job: nothing was waited for
	c, _ := New(Config{})
	if _, err := c.SubmitChallenge(context.Background(), &ChallengeInput{URL: "https://example.com/"}); err == nil || !strings.Contains(err.Error(), "proxy") {
		t.Errorf("a challenge without a proxy is refused before any call: %v", err)
	}
	if _, err := c.SubmitTurnstile(context.Background(), &TurnstileInput{PageURL: "https://example.com/", Proxy: "8.8.8.8:80"}); err == nil {
		t.Error("a widget without a site key is refused before any call")
	}
}

func TestWaitSurvivesABadPollAndStopsWithItsContext(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch polls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = io.WriteString(w, doneJob)
		}
	}))
	defer srv.Close()
	c, _ := New(Config{Endpoint: srv.URL})
	res, err := c.Wait(context.Background(), &Job{ID: "job_1", State: StateQueued})
	if err != nil || res.Clearance() != "abc123" || polls.Load() != 2 {
		t.Fatalf("%+v %v after %d polls", res, err, polls.Load())
	}

	// a job that never ends: the context ends the wait
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"job_1","state":"running"}`)
	}))
	defer hang.Close()
	c, _ = New(Config{Endpoint: hang.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Wait(ctx, &Job{ID: "job_1", State: StateRunning}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("took %s", time.Since(start))
	}
}

func TestNotFoundIsNotRetried(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polls.Add(1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"not found"}`)
	}))
	defer srv.Close()
	c, _ := New(Config{Endpoint: srv.URL})
	_, err := c.Wait(context.Background(), &Job{ID: "job_x", State: StateRunning})
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != 404 || polls.Load() != 1 {
		t.Errorf("%v after %d polls", err, polls.Load())
	}
}

func TestClearanceFromCookiesWhenThePayloadIsOther(t *testing.T) {
	r := &Result{Payload: "x", Cookies: []Cookie{{Name: "cf_clearance", Value: "from-cookie"}}}
	if r.Clearance() != "from-cookie" || (*Result)(nil).Clearance() != "" || (*Result)(nil).Token() != "" {
		t.Errorf("clearance %q", r.Clearance())
	}
}
