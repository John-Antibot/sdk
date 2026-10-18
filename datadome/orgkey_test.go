package datadome

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKeysAreSentAsHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(Config{Endpoint: srv.URL, APIKey: "ja_live_x", OrgKey: "ja_org_y"})
	_ = c.post(nil, "/x", map[string]string{}, &map[string]any{})
	if got.Get("x-api-key") != "ja_live_x" || got.Get("x-org-key") != "ja_org_y" {
		t.Fatalf("headers = %v", got)
	}
}

func TestLargeBodiesAreGzipped(t *testing.T) {
	var enc string
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enc = r.Header.Get("content-encoding")
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		b, _ := io.ReadAll(zr)
		n = len(b)
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(Config{Endpoint: srv.URL})
	_ = c.post(nil, "/x", map[string]string{"script": strings.Repeat("a", 100<<10)}, &map[string]any{})
	if enc != "gzip" || n < 100<<10 {
		t.Fatalf("encoding %q, decoded %d bytes", enc, n)
	}
}
