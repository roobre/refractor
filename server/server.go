package server

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"roob.re/refractor/names"
	"roob.re/refractor/pool"
	"roob.re/refractor/provider/providers"
	"roob.re/refractor/provider/types"
	"roob.re/refractor/stats"
	"time"
)

type Config struct {
	// Workers is the amount of workers that will serve requests in parallel. It should be higher that the amount of
	// expected connections to refractor, otherwise requests will be serialized.
	Workers int `yaml:"workers"`

	// Retries controls how many times a request is re-enqueued after a retryable error occurs.
	// Errors are considered retryable if they occur before writing anything to the client.
	Retries int `yaml:"retries"`

	// GoodThroughputMiBs is an absolute value, in MiB/s, of what is considered good throughput. Workers that perform
	// above this throughput will never get rotated out of the worker pool, even if they are in the last 2 positions.
	GoodThroughputMiBs float64 `yaml:"goodThroughputMiBs"`

	// PeekSizeBytes is the amount of bytes to peek before starting to feed the response back to the client.
	// If PeekSizeBytes are not transferred within PeekTimeout, the request is aborted and requeued to another mirror.
	PeekSizeMiBs float64 `yaml:"peekSizeMiBs"`
	// PeekTimeout is the amount of time to give for PeekSizeBytes to be read before switching to another mirror.
	PeekTimeout time.Duration `yaml:"peekTimeout"`

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

		break
	}

	if config.PeekSizeMiBs == 0 {
		log.Infof("Defaulting PeekSizeMiBs to %.1f", defaultPeekSizeMiBs)
		config.PeekSizeMiBs = defaultPeekSizeMiBs
	}

	if config.PeekTimeout == 0 {
		log.Infof("Defaulting PeekTimeout to %s", defaultPeekTimeout)
		config.PeekTimeout = defaultPeekTimeout
	}

	if config.Retries == 0 {
		log.Infof("Defaulting Retries to %s", defaultRetries)
		config.Retries = defaultRetries
	}

	s := &Server{
		pool: pool.New(pool.Config{
			PeekTimeout:   config.PeekTimeout,
			PeekSizeBytes: int64(config.PeekSizeMiBs * 1024 * 1024),
			Workers:       config.Workers,
			Namer:         names.Haiku,
			Stats: stats.New(stats.Config{
				AbsoluteGoodThroughput: config.GoodThroughputMiBs * 1024 * 1024,
				NumWorkers:             config.Workers,
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
