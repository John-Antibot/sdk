package kasada

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func TestGeneratePayload(t *testing.T) {
	var seenPath, seenKey string
	var seenBody map[string]any
	client, err := New(Config{APIKey: "key", HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenPath, seenKey = req.URL.Path, req.Header.Get("x-api-key")
		data, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(data, &seenBody)
		return &http.Response{
			StatusCode: 200, Header: http.Header{"content-type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"payloadBase64":"AAEC","headers":{"x-kpsdk-ct":"ct"},"tlUrl":"https://example.com/tl","session":"s1","userAgent":"UA"}`)),
		}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := client.GeneratePayload(context.Background(), &PayloadInput{
		Script: "ips", IpsLink: "/a/b/ips.js", PageURL: "https://example.com/", UserAgent: "UA", IP: "203.0.113.1", AcceptLanguage: "en-US",
	})
	if err != nil {
		t.Fatal(err)
	}
	if seenPath != "/v1/kasada/payload" || seenKey != "key" || seenBody["ip"] != "203.0.113.1" {
		t.Fatalf("request path=%s key=%s body=%v", seenPath, seenKey, seenBody)
	}
	if string(out.Payload) != "\x00\x01\x02" || out.HTTPHeaders().Get("x-kpsdk-ct") != "ct" {
		t.Fatalf("result = %+v", out)
	}
}

func TestVersion(t *testing.T) {
	var seenPath string
	client, _ := New(Config{HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenPath = req.URL.Path
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"version":"j-1.2.1020","payload":"j-1.2.1020","session":"s1","userAgent":"UA"}`)),
		}, nil
	})})
	out, err := client.Version(context.Background(), &VersionInput{Script: "pjs", PageURL: "https://example.com/"})
	if err != nil {
		t.Fatal(err)
	}
	if seenPath != "/v1/kasada/version" || out.Version != "j-1.2.1020" {
		t.Fatalf("path=%s out=%+v", seenPath, out)
	}
}

func TestGeneratePayloadError(t *testing.T) {
	client, _ := New(Config{HTTPClient: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 402, Body: io.NopCloser(strings.NewReader(`{"error":"balance"}`))}, nil
	})})
	_, err := client.GeneratePayload(context.Background(), &PayloadInput{
		Script: "ips", IpsLink: "/x", PageURL: "https://e/", UserAgent: "UA", IP: "1.2.3.4", AcceptLanguage: "en",
	})
	if !IsInsufficientBalance(err) {
		t.Fatalf("err = %v", err)
	}
}

const nikeKey = "1760fa26d0fa116e2f26e90dc8b1379c3d9e946f650852b7f9925457ab77a14a"

// Two cds real Chrome stamped on nike's cart writes (2026-11-24), with the x-kpsdk-ct each request carried.
func TestCDAnswersMatchRealChrome(t *testing.T) {
	for _, c := range []struct {
		first string
		want  []int
	}{
		{"tp-v2-input045g90GkupSRHwee, 1795536575353, ce1a0639146179bcf6ab456ac1925359, " + nikeKey, []int{13, 5}},
		{"tp-v2-input01pDYjEopgLXL7X4, 1795536579918, 2d455b029dc31363685cb443dce15ea4, " + nikeKey, []int{5, 1}},
	} {
		if got := cdAnswers(c.first, 10, 2); len(got) != 2 || got[0] != c.want[0] || got[1] != c.want[1] {
			t.Fatalf("cdAnswers(%q) = %v, want %v", c.first, got, c.want)
		}
	}
}

func TestCDHeader(t *testing.T) {
	v := &VersionResult{Version: "j-1.2.1020", CD: &CDParams{Difficulty: 10, SubchallengeCount: 2, CryptoCustomerKey: nikeKey, Label: "tp-v2-input", CtPrefix: true}}
	ct := "045g90GkupSRHweeAkVbaHmX2luTJgqAGpoWX3q5D4o"
	st := time.Now().UnixMilli() - 300
	var d int64
	for i := 0; i < 20; i++ {
		h, err := v.CDHeader(ct, st)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			WorkTime int64   `json:"workTime"`
			ID       string  `json:"id"`
			Answers  []int   `json:"answers"`
			Duration float64 `json:"duration"`
			D        int64   `json:"d"`
			ST       int64   `json:"st"`
			RST      int64   `json:"rst"`
		}
		if err := json.Unmarshal([]byte(h), &got); err != nil {
			t.Fatal(err)
		}
		first := fmt.Sprintf("tp-v2-input%s, %d, %s, %s", ct[:16], got.WorkTime, got.ID, nikeKey)
		if want := cdAnswers(first, 10, 2); got.Answers[0] != want[0] || got.Answers[1] != want[1] {
			t.Fatalf("answers %v do not solve their own input (%v)", got.Answers, want)
		}
		if got.ST != st || got.RST-got.ST != got.D || got.D < -10 || got.D > 80 || got.Duration < 20 || got.Duration > 75 || len(got.ID) != 32 {
			t.Fatalf("cd = %s", h)
		}
		if i == 0 {
			d = got.D
		} else if got.D != d {
			t.Fatalf("d changed within a session: %d then %d", d, got.D)
		}
		if h[:11] != `{"workTime"` {
			t.Fatalf("field order: %s", h)
		}
	}
	if _, err := (&VersionResult{}).CDHeader(ct, st); err == nil {
		t.Fatal("a version without cd parameters made a header")
	}
}

func TestVersionReadsCDParams(t *testing.T) {
	client, _ := New(Config{HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(
			`{"version":"j-1.2.1020","payload":"j-1.2.1020","cd":{"difficulty":10,"subchallengeCount":2,"cryptoCustomerKey":"k","seedSuffix":"","label":"tp-v2-input","ctPrefix":true},"session":"s1","userAgent":"UA"}`))}, nil
	})})
	out, err := client.Version(context.Background(), &VersionInput{Script: "pjs", PageURL: "https://example.com/"})
	if err != nil {
		t.Fatal(err)
	}
	if out.CD == nil || out.CD.Difficulty != 10 || out.CD.SubchallengeCount != 2 || out.CD.CryptoCustomerKey != "k" || !out.CD.CtPrefix {
		t.Fatalf("cd = %+v", out.CD)
	}
}

func TestGeneratePayloadWithoutUserAgent(t *testing.T) {
	var seenBody map[string]any
	client, _ := New(Config{HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(data, &seenBody)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(
			`{"payloadBase64":"AAEC","headers":{"x-kpsdk-ct":"ct"},"tlUrl":"https://example.com/tl","session":"s1","userAgent":"Chrome 158 as the service picked it"}`))}, nil
	})})
	out, err := client.GeneratePayload(context.Background(), &PayloadInput{
		Script: "ips", IpsLink: "/a/b/ips.js", PageURL: "https://example.com/", IP: "203.0.113.1", AcceptLanguage: "en-US",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, sent := seenBody["userAgent"]; sent {
		t.Fatalf("an empty userAgent was sent: %v", seenBody)
	}
	if out.UserAgent != "Chrome 158 as the service picked it" {
		t.Fatalf("result = %+v", out)
	}
}
