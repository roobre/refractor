package server

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"roob.re/shatter/pool"
	"roob.re/shatter/provider/providers"
	"roob.re/shatter/provider/types"
	"roob.re/shatter/stats"
)

type Config struct {
	// Workers is the amount of workers that will serve requests in parallel. It should be higher that the amount of
	// expected connections to Shatter, otherwise requests will be serialized.
	Workers int `yaml:"workers"`

	// GoodThroughputMiBs is an absolute value, in MiB/s, of what is considered good throughput. Workers that perform
	// above this throughput will never get rotated out of the worker pool, even if they are in the last 2 positions.
	GoodThroughputMiBs float64 `yaml:"goodThroughputMiBs"`

	// Provider contains the name of the chosen provider, and provider-specific config.
	Provider map[string]yaml.Node
}

type Server struct {
	pool     *pool.Pool
	provider types.Provider
}

func New(configFile io.Reader) (*Server, error) {
	config := Config{}
	err := yaml.NewDecoder(configFile).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	var provider types.Provider
	for pName, yamlConfig := range config.Provider {
		pBuilder, found := providers.Map[pName]
		if !found {
			return nil, fmt.Errorf("unknown provider %q", pName)
		}

		pConfig := pBuilder.DefaultConfig()
		err := yamlConfig.Decode(pConfig)
		if err != nil {
			return nil, fmt.Errorf("unmarshalling config for provider %q: %w", pName, err)
		}

		provider, err = pBuilder.New(pConfig)
		if err != nil {
			return nil, fmt.Errorf("creating provider %q: %w", pName, err)
		}

		break
	}

	s := &Server{
		pool: pool.New(pool.Config{
			Workers: config.Workers,
			Stats: stats.New(stats.Config{
				AbsoluteGoodThroughput: config.GoodThroughputMiBs,
			}),
		}),
	}

	s.provider = provider

	return s, nil
}

func (s *Server) Run(address string) error {
	go s.pool.Run()
	go s.pool.Feed(s.provider)

	log.Infof("Listening on %s", address)
	return http.ListenAndServe(address, s.pool)
}
