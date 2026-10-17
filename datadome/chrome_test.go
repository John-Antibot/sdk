package datadome

import "testing"

// Real Chrome headers (HAR captures and published values).
func TestSecCHUAMatchesChrome(t *testing.T) {
	for major, want := range map[int]string{
		131: `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		151: `"Not=A?Brand";v="99", "Google Chrome";v="151", "Chromium";v="151"`,
		154: `"Chromium";v="154", "Google Chrome";v="154", "Not A(Brand";v="99"`,
	} {
		if got := SecCHUA(ChromeUserAgent(major)); got != want {
			t.Errorf("Chrome %d: got %s, want %s", major, got, want)
		}
	}
	if SecCHUA("Mozilla/5.0 (Macintosh) Version/18.0 Safari/605.1.15") != "" {
		t.Error("non-Chrome UA must not get Chrome client hints")
	}
	if ChromeMajor(LatestUserAgent) != LatestChromeMajor || SecCHUAPlatform(LatestUserAgent) != `"macOS"` {
		t.Error("LatestUserAgent must be macOS Chrome LatestChromeMajor")
	}
}
