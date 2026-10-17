package datadome

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// LatestChromeMajor is the Chrome major version default identities use.
const LatestChromeMajor = 155

// LatestUserAgent is Chrome LatestChromeMajor on macOS.
var LatestUserAgent = ChromeUserAgent(LatestChromeMajor)

// ChromeUserAgent is Chrome's reduced macOS User-Agent for a major version.
func ChromeUserAgent(major int) string {
	return fmt.Sprintf("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.0.0 Safari/537.36", major)
}

var chromeMajorRe = regexp.MustCompile(`Chrome/(\d+)\.`)

// ChromeMajor is the Chrome major version in userAgent, or 0 if not Chrome.
func ChromeMajor(userAgent string) int {
	m := chromeMajorRe.FindStringSubmatch(userAgent)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// SecCHUA is the sec-ch-ua header Chrome sends for userAgent, or "" if not Chrome.
func SecCHUA(userAgent string) string {
	major := ChromeMajor(userAgent)
	if major == 0 {
		return ""
	}
	chars := []string{" ", "(", ":", "-", ".", "/", ")", ";", "=", "?", "_"}
	grease := fmt.Sprintf(`"Not%sA%sBrand";v="%s"`, chars[major%11], chars[(major+1)%11], []string{"8", "99", "24"}[major%3])
	chromium := fmt.Sprintf(`"Chromium";v="%d"`, major)
	chrome := fmt.Sprintf(`"Google Chrome";v="%d"`, major)
	order := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}[major%6]
	var list [3]string
	list[order[0]] = grease
	list[order[1]] = chromium
	list[order[2]] = chrome
	return list[0] + ", " + list[1] + ", " + list[2]
}

// SecCHUAPlatform is the quoted sec-ch-ua-platform for userAgent.
func SecCHUAPlatform(userAgent string) string {
	for _, p := range [][2]string{
		{"Windows NT", `"Windows"`}, {"Macintosh", `"macOS"`}, {"Android", `"Android"`},
		{"CrOS", `"Chrome OS"`}, {"Linux", `"Linux"`},
	} {
		if strings.Contains(userAgent, p[0]) {
			return p[1]
		}
	}
	return ""
}
