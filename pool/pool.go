package pool

import (
	"github.com/moby/moby/pkg/namesgenerator"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"roob.re/shatter/client"
	"roob.re/shatter/provider/types"
	"roob.re/shatter/stats"
	"roob.re/shatter/worker"
	"strings"
	"time"
)

type Pool struct {
	numWorkers int
	stats      *stats.Stats
	clients    chan *client.Client
	requests   chan client.Request
}

func New(workers int) *Pool {
	return &Pool{
		numWorkers: workers,
		stats:      stats.New(),
		clients:    make(chan *client.Client),
		requests:   make(chan client.Request),
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
	for i := 0; i < p.numWorkers; i++ {
		log.Debugf("Starting worker manager thread #%d", i)
		go p.work()
	}
}

func (p *Pool) work() {
	for cli := range p.clients {
		worker := worker.Worker{
			Name:   namesgenerator.GetRandomName(0),
			Client: cli,
			Stats:  p.stats,
		}
		log.Debugf("Starting worker %s for %s", worker.Name, worker.Client.String())
		log.Error(worker.Work(p.requests))
		p.stats.Remove(worker.String())
	}
}

func (p *Pool) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	responseChan := make(chan client.Response)
	request := client.Request{
		Path:         r.URL.Path,
		ResponseChan: responseChan,
	}

	retries := 0
	for {
		if retries > 3 {
			log.Errorf("Max retries for %s exhausted", r.URL.Path)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		log.Debugf("Dispatching request %q to workers", r.URL.Path)
		p.requests <- request
		response := <-responseChan
		if response.Error != nil {
			log.Warnf("Worker returned an error for %q, requeuing: %v", r.URL.Path, response.Error)
			retries++
			continue
		}

		if response.HTTPResponse.StatusCode != http.StatusOK {
			// TODO: Hack: Archlinux mirrors are somehow expected to return 404 for .sig files.
			// For this reason, we do not attempt to retry 404s for .sig files.
			if !strings.HasSuffix(r.URL.Path, ".sig") {
				log.Warnf("Worker returned an error for %q, requeuing: %v", r.URL.Path, response.Error)
				retries++
				continue
			}
		}

		//maps.Copy(rw.Header(), response.HTTPResponse.Header)
		rw.WriteHeader(response.HTTPResponse.StatusCode)
		_, err := io.Copy(rw, response.HTTPResponse.Body)
		if err != nil {
			log.Errorf("Could not write response to %s back to client: %v", r.URL.Path, err)
			return
		}
		response.Done()

		return
	}
}
