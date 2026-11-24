package kasada

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
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
