// Package jar is a minimal insertion-ordered cookie jar for the examples.
package jar

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type entry struct {
	name, value, domain string
}

// Jar stores cookies in insertion order.
type Jar struct {
	mu      sync.Mutex
	cookies []entry
	idx     map[string]int // domain\x00name -> position
}

// New returns an empty Jar.
func New() *Jar { return &Jar{idx: map[string]int{}} }

// Set stores one cookie for domain and its subdomains.
func (j *Jar) Set(domain, name, value string) {
	domain = strings.TrimPrefix(strings.ToLower(domain), ".")
	j.mu.Lock()
	defer j.mu.Unlock()
	key := domain + "\x00" + name
	if i, ok := j.idx[key]; ok {
		j.cookies[i].value = value
		return
	}
	j.idx[key] = len(j.cookies)
	j.cookies = append(j.cookies, entry{name, value, domain})
}

// StoreResponseCookies stores the Set-Cookie lines of a response from rawURL.
func (j *Jar) StoreResponseCookies(rawURL string, header http.Header) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	for _, line := range header.Values("Set-Cookie") {
		c, err := http.ParseSetCookie(line)
		if err != nil {
			continue
		}
		domain := c.Domain
		if domain == "" {
			domain = u.Hostname()
		}
		j.Set(domain, c.Name, c.Value)
	}
}

func (j *Jar) matching(rawURL string) []entry {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []entry
	for _, c := range j.cookies {
		if host == c.domain || strings.HasSuffix(host, "."+c.domain) {
			out = append(out, c)
		}
	}
	return out
}

// Header returns the Cookie header value for rawURL.
func (j *Jar) Header(rawURL string) string {
	parts := []string{}
	for _, c := range j.matching(rawURL) {
		parts = append(parts, c.name+"="+c.value)
	}
	return strings.Join(parts, "; ")
}

// Get returns one stored cookie value for rawURL.
func (j *Jar) Get(rawURL, name string) string {
	for _, c := range j.matching(rawURL) {
		if c.name == name {
			return c.value
		}
	}
	return ""
}
