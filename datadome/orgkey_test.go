package datadome

import (
	"net/http"
	"net/http/httptest"
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
