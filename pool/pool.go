package pool

import (
	"errors"
	"fmt"
	"github.com/moby/moby/pkg/namesgenerator"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"roob.re/refractor/client"
	"roob.re/refractor/provider/types"
	"roob.re/refractor/stats"
	"roob.re/refractor/worker"
	"strings"
	"time"
)

type Pool struct {
	Config
	clients  chan *client.Client
	requests chan client.Request
}

type Config struct {
	Workers       int
	Stats         *stats.Stats
	PeekSizeBytes int64
	PeekTimeout   time.Duration
}

func New(config Config) *Pool {
	return &Pool{
		Config:   config,
		clients:  make(chan *client.Client),
		requests: make(chan client.Request),
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
			Name:   namesgenerator.GetRandomName(0),
			Client: cli,
			Stats:  p.Stats,
		}
		log.Debugf("Starting worker %s for %s", worker.Name, worker.Client.String())
		log.Error(worker.Work(p.requests))
		p.Stats.Remove(worker.String())
	}
}

func (p *Pool) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	retries := 0
	for {
		if retries > 3 {
			log.Errorf("Max retries for %s exhausted", r.URL.Path)
			break
		}

		err, retryable := p.tryRequest(r, rw)
		if err != nil {
			log.Errorf("%v", err)
			if !retryable {
				break
			}
		}

		retries++
	}

	rw.WriteHeader(http.StatusInternalServerError)
	return
}

func (p *Pool) tryRequest(r *http.Request, rw http.ResponseWriter) (error, bool) {
	responseChan := make(chan client.Response)
	request := client.Request{
		Path:         r.URL.Path,
		ResponseChan: responseChan,
	}

	log.Debugf("Dispatching request %s to workers", request.Path)
	p.requests <- request
	response := <-responseChan
	if response.Error != nil {
		return fmt.Errorf("%s%s errored: %w", response.Worker, request.Path, response.Error), true
	}

	if response.HTTPResponse.StatusCode != http.StatusOK {
		// TODO: Hack: Archlinux mirrors are somehow expected to return 404 for .sig files.
		// For this reason, we do not attempt to retry 404s for .sig files.
		if !strings.HasSuffix(r.URL.Path, ".sig") {
			return fmt.Errorf("%s%s returned non-200 status: %d", response.Worker, request.Path, response.HTTPResponse.StatusCode), true
		}
	}

	retryable := false
	written, err := p.writeResponse(response.HTTPResponse, rw)
	response.Done(written)
	if errors.Is(err, errPeek) {
		// Peek errors are retryable as we haven't written anything to the client yet
		err = fmt.Errorf("peek timed out: %w", err)
		retryable = true
	}

	if err != nil {
		err = fmt.Errorf("writing request %s%s to client: %w", response.Worker, request.Path, err)
		return err, retryable
	}

	return nil, false
}

func (p *Pool) writeResponse(response *http.Response, rw http.ResponseWriter) (int64, error) {
	peeker := Peeker{
		SizeBytes: p.PeekSizeBytes,
		Timeout:   p.PeekTimeout,
	}
	// Peek body before writing headers
	peeked, err := peeker.Peek(response.Body)
	if err != nil {
		_ = response.Body.Close()
		return 0, fmt.Errorf("%w: %v", errPeek, err)
	}

	for header, name := range response.Header {
		rw.Header().Set(header, name[0])
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
