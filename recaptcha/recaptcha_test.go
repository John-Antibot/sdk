package recaptcha

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

const doneJob = `{"id":"job_1","state":"done","task":"v3","createdAt":"2026-10-23T10:00:00Z","proxy":"8.8.8.8:80","result":{"token":"03AFcWeA","expiresAt":"2026-10-23T10:01:50.000Z",` +
	`"userAgent":"UA/1","acceptLanguage":"en-US,en;q=0.9","secUa":"\"Chromium\";v=\"156\"","secUaMobile":"?0","secUaPlatform":"\"macOS\""}}`

// api is a stand-in for the API: submit answers 202 with job_1, the first poll says running, the next says terminal.
type api struct {
	t         *testing.T
	terminal  string
	route     string
	submitted map[string]any
	polls     atomic.Int32
	waits     []string
}

func (a *api) handler() http.Handler {
	mux := http.NewServeMux()
	submit := func(route string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-api-key") != "key" || r.Header.Get("x-org-key") != "org" {
				a.t.Errorf("headers: key %q org %q", r.Header.Get("x-api-key"), r.Header.Get("x-org-key"))
			}
			a.route = route
			b, _ := io.ReadAll(r.Body)
			a.submitted = map[string]any{}
			_ = json.Unmarshal(b, &a.submitted)
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"id":"job_1","state":"queued"}`)
		}
	}
	mux.HandleFunc("POST /v3", submit("v3"))
	mux.HandleFunc("POST /v3-enterprise", submit("v3-enterprise"))
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

func TestSolveV3SubmitsAndWaits(t *testing.T) {
	a := &api{t: t, terminal: doneJob}
	c := newClient(t, a)
	res, err := c.SolveV3(context.Background(), &Input{
		URL: "https://example.com/login", SiteKey: "6Lc-key_1", Action: "login", Proxy: "u:p@8.8.8.8:80", Timeout: 45 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "03AFcWeA" || res.UserAgent != "UA/1" || res.AcceptLanguage != "en-US,en;q=0.9" || res.SecUaMobile != "?0" || res.SecUaPlatform != `"macOS"` {
		t.Errorf("result = %+v", res)
	}
	if want := time.Date(2026, 10, 23, 10, 1, 50, 0, time.UTC); !res.ExpiresAt.Equal(want) {
		t.Errorf("expiresAt = %v, want %v", res.ExpiresAt, want)
	}
	// what was sent, and to the classic route
	want := map[string]any{"url": "https://example.com/login", "siteKey": "6Lc-key_1", "action": "login", "proxy": "u:p@8.8.8.8:80", "timeoutMs": float64(45000)}
	if a.route != "v3" || len(a.submitted) != len(want) {
		t.Errorf("route %q submitted %v", a.route, a.submitted)
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

func TestEnterpriseGoesToItsRouteAndLeavesEmptyOptionalsOut(t *testing.T) {
	a := &api{t: t, terminal: strings.Replace(doneJob, `"task":"v3"`, `"task":"v3-enterprise"`, 1)}
	c := newClient(t, a)
	res, err := c.SolveEnterprise(context.Background(), &Input{URL: "https://example.com/", SiteKey: "6Lc-key_1", Proxy: "8.8.8.8:80"})
	if err != nil || res.Token != "03AFcWeA" {
		t.Fatalf("%+v %v", res, err)
	}
	// the API's own defaults stand for what was not given: neither is sent
	if a.route != "v3-enterprise" || a.submitted["action"] != nil || a.submitted["timeoutMs"] != nil || len(a.submitted) != 3 {
		t.Errorf("route %q submitted %v", a.route, a.submitted)
	}
}

func TestEndedJobsAreErrors(t *testing.T) {
	for _, tc := range []struct {
		name, terminal, message string
		declined, retry         bool
	}{
		{"declined", `{"id":"job_1","state":"declined","error":"the attempt was turned down","retry":true}`, "the attempt was turned down", true, true},
		{"failed", `{"id":"job_1","state":"failed","error":"the proxy is refused: x is not a public address"}`, "the proxy is refused: x is not a public address", false, false},
		{"result expired", `{"id":"job_1","state":"done","resultExpired":true}`, "no longer holds its result", false, false},
		{"done without a result", `{"id":"job_1","state":"done"}`, "without a result", false, false},
	} {
		a := &api{t: t, terminal: tc.terminal}
		_, err := newClient(t, a).SolveV3(context.Background(), &Input{URL: "https://example.com/", SiteKey: "k", Proxy: "8.8.8.8:80"})
		var job *JobError
		if !errors.As(err, &job) || IsDeclined(err) != tc.declined || job.Retry != tc.retry || job.ID != "job_1" || !strings.Contains(job.Message, tc.message) {
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
		_, err := c.SolveV3(context.Background(), &Input{URL: "https://example.com/", SiteKey: "k", Proxy: "8.8.8.8:80"})
		srv.Close()
		var api *APIError
		if !errors.As(err, &api) || api.StatusCode != tc.status || api.Message != "gateway: nope" || !tc.check(err) {
			t.Errorf("%d: %v", tc.status, err)
		}
	}
	// a solve with a field missing is refused before any call
	c, _ := New(Config{})
	for name, in := range map[string]*Input{
		"no proxy":    {URL: "https://example.com/", SiteKey: "k"},
		"no site key": {URL: "https://example.com/", Proxy: "8.8.8.8:80"},
		"no url":      {SiteKey: "k", Proxy: "8.8.8.8:80"},
		"nil":         nil,
	} {
		if _, err := c.SubmitV3(context.Background(), in); err == nil {
			t.Errorf("%s: refused before any call", name)
		}
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
	if err != nil || res.Token != "03AFcWeA" || polls.Load() != 2 {
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

func TestHeadersAreTheBrowserTheTokenWasMadeAs(t *testing.T) {
	var job Job
	if err := json.Unmarshal([]byte(doneJob), &job); err != nil {
		t.Fatal(err)
	}
	h := job.Result.Headers()
	if h.Get("user-agent") != "UA/1" || h.Get("accept-language") != "en-US,en;q=0.9" || h.Get("sec-ch-ua") != `"Chromium";v="156"` ||
		h.Get("sec-ch-ua-mobile") != "?0" || h.Get("sec-ch-ua-platform") != `"macOS"` {
		t.Errorf("headers = %v", h)
	}
	// a result without client hints (a browser that sends none) leaves them out instead of sending empty ones; a nil result has none at all
	bare := (&Result{UserAgent: "UA/2", AcceptLanguage: "de"}).Headers()
	if len(bare) != 2 || bare.Get("sec-ch-ua") != "" || len((*Result)(nil).Headers()) != 0 {
		t.Errorf("bare headers = %v", bare)
	}
	// a result with no expiry says so with the zero time
	var open Result
	if err := json.Unmarshal([]byte(`{"token":"t","expiresAt":null}`), &open); err != nil || !open.ExpiresAt.IsZero() {
		t.Errorf("%+v %v", open, err)
	}
}
