package archlinux

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"net/http"
	"roob.re/refractor/provider/types"
	"strings"
	"time"
)

const mirrorsUrl = "https://archlinux.org/mirrors/status/json/"

type config struct {
	CountriesList []string `yaml:"countries"`
	MaxScore      float64  `yaml:"maxScore"`

	countries map[string]bool
}

type Provider struct {
	config

	mirrorlist struct {
		list    []mirror
		fetched time.Time
	}
}

func New(conf interface{}) (types.Provider, error) {
	acConfig, ok := conf.(*config)
	if !ok {
		return nil, fmt.Errorf("internal error: supplied config is not of the expected type")
	}

	// Convert country list (human friendly) into map (code friendly)
	acConfig.countries = map[string]bool{}
	for _, country := range acConfig.CountriesList {
		acConfig.countries[country] = true
	}

	return &Provider{
		config: *acConfig,
	}, nil
}

func DefaultConfig() interface{} {
	return &config{}
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

		if len(a.countries) > 0 && !a.countries[mirror.Country] {
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
