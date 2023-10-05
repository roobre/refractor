package refractor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"roob.re/refractor/pool"
	"roob.re/refractor/stats"
)

type Refractor struct {
	Config
	Pool *pool.Pool

	buffers sync.Pool
}

type Config struct {
	ChunkSizeMiBs int           `yaml:"chunkSizeMiBs"`
	ChunkTimeout  time.Duration `yaml:"chunkTimeout"`
	Retries       int           `yaml:"retries"`
}

func (c Config) WithDefaults() Config {
	const (
		defaultChunkSize    = 4
		defaultChunkTimeout = 5 * time.Second
		defaultRetries      = 5
	)

	if c.ChunkSizeMiBs == 0 {
		c.ChunkSizeMiBs = defaultChunkSize
	}

	if c.ChunkTimeout == 0 {
		c.ChunkTimeout = defaultChunkTimeout
	}

	if c.Retries == 0 {
		c.Retries = defaultRetries
	}

	return c
}

func New(c Config, pool *pool.Pool) *Refractor {
	return &Refractor{
		Config: c.WithDefaults(),
		Pool:   pool,
		buffers: sync.Pool{
			New: func() any {
				return &bytes.Buffer{}
			},
		},
	}
}

type responseErr struct {
	err      error
	response *http.Response
}

func (rf *Refractor) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	url := r.URL.String()

	// Archlinux hack: Mirrors return 404 for .db.sig files.
	// TODO: Mirror-specific hacks should be a on a different, possibly config-driven object that wraps Refractor.
	if strings.HasSuffix(url, ".db.sig") {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	// Archlinux quirk: .db files change very often between mirrors, splitting them is almost guaranteed to return a
	// corrupted file, so they are handled to a single mirror.
	if strings.HasSuffix(url, ".db") {
		rf.handlePlain(rw, r)
		return
	}

	// Other requests are refracted across mirrors.
	rf.handleRefracted(rw, r)
}

func (rf *Refractor) handlePlain(rw http.ResponseWriter, r *http.Request) {
	url := r.URL.String()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Errorf("building GET request for %q: %v", url, err)
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	br := <-rf.retryRequest(req)
	if br.err != nil {
		log.Errorf("GET request for %q failed: %v", url, err)
		rw.WriteHeader(http.StatusBadGateway)
		return
	}

	for k, vs := range br.response.Header {
		for _, v := range vs {
			rw.Header().Add(k, v)
		}
	}

	_, err = io.Copy(rw, br.response.Body)
	if err != nil {
		log.Errorf("writing GET body: %v", err)
	}
}

func (rf *Refractor) handleRefracted(rw http.ResponseWriter, r *http.Request) {
	url := r.URL.String()

	headReq, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		log.Errorf("building HEAD request for %q: %v", url, err)
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	headReq.Header.Add("accept-encoding", "identity") // Prevent server from gzipping response.
	br := <-rf.retryRequest(headReq)

	var responseChannels []chan responseErr

	size := br.response.ContentLength
	start := int64(0)
	for start < size {
		end := start + int64(rf.ChunkSizeMiBs)<<20
		if end > size {
			end = size
		}

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			log.Errorf("building ranged retryRequest for %q: %v", url, err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		req.Header.Add("range", fmt.Sprintf("bytes=%d-%d", start, end))
		// Prevent servers from gzipping request, as that would break ranges across servers.
		req.Header.Add("accept-encoding", "identity")
		responseChannels = append(responseChannels, rf.retryRequest(req))

		start = end + 1 // Server returns [start-end], both inclusive, so next request should start on end + 1.
	}

	// Defer fully consuming and closing all response channels to avoid leaking buffers and workers, in the event an
	// error occurs.
	defer func() {
		for _, rc := range responseChannels {
			re := <-rc
			if re.response != nil {
				re.response.Body.Close()
			}
		}
	}()

	for k, vs := range br.response.Header {
		for _, v := range vs {
			rw.Header().Add(k, v)
		}
	}

	written := int64(0)
	for _, rc := range responseChannels {
		re := <-rc
		if re.err != nil {
			log.Errorf("Reading resopnse from channel: %v", err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		n, err := io.Copy(rw, re.response.Body)
		if err != nil {
			log.Errorf("Writing response chunk: %v", err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		written += n

		re.response.Body.Close()
	}

	if written != br.response.ContentLength {
		log.Errorf("Wrote %d bytes of %d expected for %s", written, br.response.ContentLength, url)
		return
	}
}

func (rf *Refractor) retryRequest(r *http.Request) chan responseErr {
	respChan := make(chan responseErr)
	go func() {
		defer close(respChan)

		retries := rf.Retries
		try := 0
		for {
			try++

			response, err := rf.request(r)
			if err != nil {
				log.Errorf("[%d/%d] Requesting %s[%s]: %v", try, retries, r.URL.Path, r.Header.Get("range"), err)
				if try < retries {
					continue
				}

				log.Errorf("Giving up on %s[%s]: %v", r.URL.Path, r.Header.Get("range"), err)

				respChan <- responseErr{
					err: err,
				}

				return
			}

			respChan <- responseErr{
				response: response,
			}

			return
		}
	}()

	return respChan
}

func (rf *Refractor) request(r *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rf.ChunkTimeout)
	defer cancel()

	r = r.WithContext(ctx)

	expectedStatus := http.StatusOK
	if r.Header.Get("range") != "" {
		expectedStatus = http.StatusPartialContent
	}

	response, err := rf.Pool.Do(r)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	if response.StatusCode != expectedStatus {
		return nil, fmt.Errorf("got status %d, expected %d", response.StatusCode, expectedStatus)
	}

	if response.ContentLength == -1 {
		return nil, fmt.Errorf("got -1 content length for response")
	}

	// If this is a HEAD request there is no need to copy the body.
	if r.Method == http.MethodHead {
		return response, nil
	}

	buf := rf.buffers.Get().(*bytes.Buffer)
	buf.Reset()

	body := response.Body

	// Asynchronously wait for context and close body if copy takes too long.
	go func() {
		<-ctx.Done()

		err := body.Close()
		if err != nil {
			log.Errorf("Closing body due to context timeout: %v", err)
		}
	}()

	n, err := io.Copy(buf, body)
	if err != nil {
		return nil, err
	}

	// Check we read the expected length.
	if n != response.ContentLength {
		return nil, fmt.Errorf("expected to read bytes %d but read %d instead", response.ContentLength, n)
	}

	response.Body = &stats.ReaderWrapper{
		Underlying: buf,
		OnDone: func(_ uint64) {
			rf.buffers.Put(buf)
		},
	}

	return response, nil
}
