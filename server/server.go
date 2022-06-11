package server

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"roob.re/refractor/client"
	"roob.re/refractor/pool"
	"roob.re/refractor/provider/providers"
	"roob.re/refractor/provider/types"
	"roob.re/refractor/stats"
	"time"
)

type Config struct {
	Pool   pool.Config   `yaml:",inline"`
	Client client.Config `yaml:",inline"`

	GoodThroughputMiBs float64 `yaml:"goodThroughputMiBs"`

	// Provider contains the name of the chosen provider, and provider-specific config.
	Provider map[string]yaml.Node
}

const (
	defaultPeekSizeMiBs = 1.0
	defaultPeekTimeout  = 4 * time.Second
	defaultRetries      = 3
)

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

		log.Infof("Using provider %q", pName)

		break
	}

	if config.Pool.PeekSizeMiBs == 0 {
		log.Infof("Defaulting PeekSizeMiBs to %.1f", defaultPeekSizeMiBs)
		config.Pool.PeekSizeMiBs = defaultPeekSizeMiBs
	}

	if config.Pool.PeekTimeout == 0 {
		log.Infof("Defaulting PeekTimeout to %s", defaultPeekTimeout)
		config.Pool.PeekTimeout = defaultPeekTimeout
	}

	if config.Pool.Retries == 0 {
		log.Infof("Defaulting Retries to %d", defaultRetries)
		config.Pool.Retries = defaultRetries
	}

	return &Server{
		provider: provider,
		pool: pool.New(
			config.Pool,
			stats.New(stats.Config{
				AbsoluteGoodThroughput: config.GoodThroughputMiBs * 1024 * 1024,
				NumWorkers:             config.Pool.Workers,
			}),
		),
	}, nil
}

func (s *Server) Run(address string) error {
	go s.pool.Run()
	go s.pool.Feed(s.provider)

	log.Infof("Listening on %s", address)
	return http.ListenAndServe(address, s.pool)
}
