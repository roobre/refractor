package refractor_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	mrand "math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"roob.re/refractor/pool"
	"roob.re/refractor/provider/providers/fake"
	"roob.re/refractor/refractor"
	"roob.re/refractor/stats"
)

type requestJournal struct {
	entries []*http.Request
	mtx     sync.Mutex
}

func (j *requestJournal) Log(r *http.Request) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.entries = append(j.entries, r)
}

func Test_Refractor(t *testing.T) {
	journal := requestJournal{}

	rubbish, err := io.ReadAll(io.LimitReader(rand.Reader, 50<<20))
	if err != nil {
		t.Fatalf("error reading rubbish: %v", err)
	}

	mirrors := make([]string, 5)
	for i := range mirrors {
		s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			journal.Log(r)

			if mrand.Float32() < 0.2 {
				rw.WriteHeader(http.StatusGatewayTimeout)
				return
			}

			http.ServeContent(rw, r, "rubbish", time.Now(), bytes.NewReader(rubbish))
		}))

		mirrors[i] = s.URL
		t.Cleanup(func() {
			s.Close()
		})
	}

	pool := pool.New(
		fake.Fake{Mirrors: mirrors},
		stats.New(stats.Config{}),
	)

	go func() {
		err := pool.Start(context.Background())
		if err != nil {
			t.Errorf("starting pool: %v", err)
		}
	}()

	r := refractor.New(refractor.Config{}, pool)
	server := httptest.NewServer(r)

	response, err := http.Get(server.URL + "/rubbish")
	if err != nil {
		t.Fatalf("refractor returned error: %v", err)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("cannot read response body error: %v", err)
	}

	if !bytes.Equal(body, rubbish) {
		t.Fatalf("body does not equal rubbish")
	}

	if len(journal.entries) <= 1 {
		t.Fatalf("server pool did not get the expected number of requests")
	}
}
