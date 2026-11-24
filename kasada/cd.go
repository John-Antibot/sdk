package kasada

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	mathrand "math/rand/v2"
	"strconv"
	"time"
)

// CDParams is what x-kpsdk-cd is made from besides the session: the
// numbers and strings p.js's proof of work uses. Version returns them;
// they change only when the site's p.js does.
type CDParams struct {
	Difficulty        int    `json:"difficulty"`
	SubchallengeCount int    `json:"subchallengeCount"`
	CryptoCustomerKey string `json:"cryptoCustomerKey"`
	SeedSuffix        string `json:"seedSuffix"`
	Label             string `json:"label"`
	CtPrefix          bool   `json:"ctPrefix"`
}

// CDHeader makes the x-kpsdk-cd header for one protected request. It is
// computed here: about ten SHA-256 hashes, no API call, no charge.
//
// ct is the session's current x-kpsdk-ct (the one you send on this very
// request: each reply rotates it) and st is the x-kpsdk-st header /tl
// answered with, in milliseconds. Call it again for every request; each
// header is fresh.
func (r *VersionResult) CDHeader(ct string, st int64) (string, error) {
	if r == nil || r.CD == nil {
		return "", errors.New("kasada: this version has no cd parameters (p.js's proof of work could not be read)")
	}
	return r.CD.Header(ct, st)
}

// Header is CDHeader for parameters you kept.
func (p *CDParams) Header(ct string, st int64) (string, error) {
	if p == nil || p.Difficulty < 1 || p.SubchallengeCount < 1 {
		return "", errors.New("kasada: difficulty and subchallengeCount must be positive")
	}
	if st <= 0 {
		return "", errors.New("kasada: st must be the x-kpsdk-st value /tl answered with")
	}
	label := p.Label
	if label == "" {
		label = "tp-v2-input"
	}
	if p.CtPrefix {
		if len(ct) < 16 {
			return "", errors.New("kasada: ct must be the x-kpsdk-ct value (at least 16 characters)")
		}
		label += ct[:16]
	}
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	id := hex.EncodeToString(raw[:])
	workTime := time.Now().UnixMilli() - 10 - int64(mathrand.IntN(41))
	first := fmt.Sprintf("%s, %d, %s", label, workTime, id)
	if p.CryptoCustomerKey != "" {
		first += ", " + p.CryptoCustomerKey
	}
	if p.SeedSuffix != "" {
		first += ", " + p.SeedSuffix
	}
	// A real page's clock reads st a little before or after the server's: the same offset for the whole session,
	// so it comes from st itself and every request of one session carries the same d.
	d := int64(sha256.Sum256([]byte(strconv.FormatInt(st, 10)))[0])*90/255 - 10
	out, err := json.Marshal(cdHeader{
		WorkTime: workTime, ID: id, Answers: cdAnswers(first, p.Difficulty, p.SubchallengeCount),
		Duration: float64(200+mathrand.IntN(551)) / 10, D: d, ST: st, RST: st + d,
	})
	return string(out), err
}

// cdHeader is the header's JSON; the field order is the wire order.
type cdHeader struct {
	WorkTime int64   `json:"workTime"`
	ID       string  `json:"id"`
	Answers  []int   `json:"answers"`
	Duration float64 `json:"duration"`
	D        int64   `json:"d"`
	ST       int64   `json:"st"`
	RST      int64   `json:"rst"`
}

// cdAnswers is p.js's proof of work: for each round, the first nonce n
// whose hash sha256("<n>, <previous hash>") scores at least
// difficulty/rounds, where the score is 2^52 over the first 13 hex digits
// (plus one). The passing hash starts the next round.
func cdAnswers(first string, difficulty, rounds int) []int {
	target := float64(difficulty) / float64(rounds)
	hash := sha256Hex(first)
	answers := make([]int, 0, rounds)
	for len(answers) < rounds {
		for n := 1; ; n++ {
			next := sha256Hex(strconv.Itoa(n) + ", " + hash)
			prefix, _ := strconv.ParseUint(next[:13], 16, 64)
			if float64(1<<52)/float64(prefix+1) >= target {
				answers = append(answers, n)
				hash = next
				break
			}
		}
	}
	return answers
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
