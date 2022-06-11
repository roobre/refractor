package pool

import (
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"roob.re/refractor/client"
	"roob.re/refractor/names"
	"roob.re/refractor/pool/peeker"
	"roob.re/refractor/provider/types"
	"roob.re/refractor/stats"
	"roob.re/refractor/worker"
	"strings"
	"time"
)

type Pool struct {
	Config
	stats  *stats.Stats
	peeker peeker.Peeker
	namer  func() string

	clients  chan *client.Client
	requests chan client.Request
}

type Config struct {
	// Retries controls how many times a request is re-enqueued after a retryable error occurs.
	// Errors are considered retryable if they occur before writing anything to the client.
	Retries int `yaml:"retries"`
	// Workers is the amount of workers that will serve requests in parallel. It should be higher that the amount of
	// expected connections to refractor, otherwise requests will be serialized.
	Workers int `yaml:"workers"`

	// PeekSizeMiBs is the amount of bytes to peek before starting to feed the response back to the client.
	// If PeekSizeMiBs are not transferred within PeekTimeout, the request is aborted and requeued to another mirror.
	PeekSizeMiBs int64 `yaml:"peekSizeMiBs"`
	// PeekTimeout is the amount of time to give for PeekSizeBytes to be read before switching to another mirror.
	PeekTimeout time.Duration `yaml:"peekTimeout"`
}

func New(config Config, stats *stats.Stats) *Pool {
	return &Pool{
		Config:   config,
		stats:    stats,
		namer:    names.Haiku,
		clients:  make(chan *client.Client),
		requests: make(chan client.Request),
		peeker: peeker.Peeker{
			SizeBytes: config.PeekSizeMiBs * 1024 * 1024,
			Timeout:   config.PeekTimeout,
		},
	}
}

func (p *Pool) Feed(provider types.Provider) {
	log.Infof("Starting to feed mirrors to the pool")
	for {
		url, err := provider.Mirror()
		if err != nil {
			log.Errorf("Provided returned an error: %v", err)
			time.Sleep(10 * time.Second)
		}
		p.clients <- client.NewClient(url)
	}
}

func (p *Pool) Run() {
	for i := 0; i < p.Workers; i++ {
		log.Debugf("Starting worker manager thread #%d", i)
		go p.work()
	}
}

func (p *Pool) work() {
	for cli := range p.clients {
		worker := worker.Worker{
			Client: cli,
			Stats:  p.stats,
			Name:   p.namer(),
		}
		log.Error(worker.Work(p.requests))
		p.stats.Remove(worker.String())
	}
}

func (p *Pool) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	retries := 0
	for {
		if retries > p.Config.Retries {
			log.Errorf("Max retries for %s exhausted", r.URL.Path)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		err, retryable := p.tryRequest(r, rw)
		if err == nil {
			return
		}

		log.Errorf("%v", err)
		if !retryable {
			return
		}

		log.Warnf("Retrying %s", r.URL.Path)
		retries++
	}
}

func (p *Pool) tryRequest(r *http.Request, rw http.ResponseWriter) (error, bool) {
	responseChan := make(chan client.Response)
	request := client.Request{
		Path:         r.URL.Path,
		ResponseChan: responseChan,
		Header:       r.Header,
	}

	log.Debugf("Dispatching request %s to workers", request.Path)
	p.requests <- request
	response := <-responseChan
	if response.Error != nil {
		return fmt.Errorf("%s%s errored: %w", response.Worker, request.Path, response.Error), true
	}

	if response.HTTPResponse.StatusCode >= 400 {
		// TODO: Hack: Archlinux mirrors are somehow expected to return 404 for .sig files.
		// For this reason, we do not attempt to retry 404s for .sig files.
		if !strings.HasSuffix(r.URL.Path, ".sig") {
			return fmt.Errorf("%s%s returned non-200 status: %d", response.Worker, request.Path, response.HTTPResponse.StatusCode), true
		}
	}

	written, err := p.writeResponse(response.HTTPResponse, rw)
	response.Done(written)

	if err != nil {
		err = fmt.Errorf("writing %s%s to client: %w", response.Worker, request.Path, err)
		return err, written == 0
	}

	return nil, false
}

func (p *Pool) writeResponse(response *http.Response, rw http.ResponseWriter) (int64, error) {
	// Peek body before writing headers
	peeked, err := p.peeker.Peek(response.Body)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return 0, fmt.Errorf("peeking response body: %w", err)
	}

	for header, values := range response.Header {
		for _, value := range values {
			rw.Header().Add(header, value)
		}
	}

	rw.WriteHeader(response.StatusCode)
	peekedWritten, err := rw.Write(peeked)
	if err != nil {
		return int64(peekedWritten), fmt.Errorf("writing peeked body: %w", err)
	}

	restWritten, err := io.Copy(rw, response.Body)
	written := int64(peekedWritten) + restWritten
	if err != nil {
		return written, fmt.Errorf("writing body: %w", err)
	}

	return written, nil
}
