package worker

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
	"roob.re/shatter/client"
	"roob.re/shatter/stats"
	"strings"
	"time"
)

type Worker struct {
	Name   string
	Stats  *stats.Stats
	Client *client.Client
}

func (w Worker) String() string {
	return fmt.Sprintf("%s:%s", w.Name, w.Client.String())
}

func (w Worker) Work(requests chan client.Request) error {
	for req := range requests {
		if !w.Stats.GoodPerformer(w.String()) {
			go func() {
				requests <- req
			}()

			return fmt.Errorf("worker %s is not a good performer, evicting and requeuing request", w.Name)
		}

		log.Debugf("%s handling %s", w.String(), req.Path)

		start := time.Now()
		response := w.Client.Do(req.Path)

		if response.Error != nil {
			go func() {
				requests <- req
			}()

			return fmt.Errorf("worker %s returned error for %s, sacrificing: %v", w.String(), req.Path, response.Error)
		}

		if code := response.HTTPResponse.StatusCode; code != http.StatusOK {
			// TODO: Hack: Archlinux mirrors are somehow expected to return 404 for .sig files.
			if !strings.HasSuffix(req.Path, ".sig") {
				log.Warnf("Worker %s returned %d for %s", w.String(), code, req.Path)
			}
		}

		response.Done = func() {
			go w.Stats.Update(w.String(), stats.Sample{
				Bytes:    response.HTTPResponse.ContentLength,
				Duration: time.Since(start),
			})
		}

		req.ResponseChan <- response
	}

	return fmt.Errorf("request channel closed")
}