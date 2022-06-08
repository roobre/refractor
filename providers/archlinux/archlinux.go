package archlinux

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

const mirrorsUrl = "https://archlinux.org/mirrors/status/json/"

type Provider struct {
	Countries map[string]bool
	MaxScore  float64

	mirrorlist struct {
		list    []mirror
		fetched time.Time
	}
}

type mirror struct {
	Score    float64 `json:"score"`
	Country  string  `json:"country_code"`
	Protocol string  `json:"protocol"`
	URL      string  `json:"url"`
}

func (m *mirror) String() string {
	return fmt.Sprintf("score=%.2f country=%s url=%s", m.Score, m.Country, m.URL)
}

func (a *Provider) filter(all []mirror) []mirror {
	list := make([]mirror, 0, len(all)/4)
	for _, mirror := range all {
		if !strings.Contains(mirror.Protocol, "http") {
			continue
		}

		if a.MaxScore > 0 && mirror.Score > a.MaxScore {
			continue
		}

		if len(a.Countries) > 0 && !a.Countries[mirror.Country] {
			continue
		}

		list = append(list, mirror)
	}

	return list
}

func (a *Provider) mirrors() ([]mirror, error) {
	if time.Since(a.mirrorlist.fetched) < time.Hour {
		return a.mirrorlist.list, nil
	}

	log.Infof("Requesting mirrorlist from %s", mirrorsUrl)
	resp, err := http.Get(mirrorsUrl)
	if err != nil {
		return nil, fmt.Errorf("fetching mirrorlist: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("wrong status code %d", resp.StatusCode)
	}

	var response struct {
		Version int      `json:"version"`
		Mirrors []mirror `json:"urls"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("decoding json: %w", err)
	}

	list := a.filter(response.Mirrors)

	a.mirrorlist.list = list
	a.mirrorlist.fetched = time.Now()

	return list, nil
}

func (a *Provider) Mirror() (string, error) {
	list, err := a.mirrors()
	if err != nil {
		return "", fmt.Errorf("accessing mirrorlist: %w", err)
	}

	mirror := list[rand.Int63n(int64(len(list)))]
	log.Infof("Mirror fed to pool: %s", mirror.String())

	return mirror.URL, nil
}
