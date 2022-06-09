package stats

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"sync"
	"time"
)

const (
	minSampleBytes = 1024
	// Requests below minSampleBytes will not be counted UNLESS they took more than minDurationForMinBytes
	minDurationForMinBytes = 1 * time.Second
	minSampleDuration      = 200 * time.Millisecond
	maxSamples             = 20.0
)

type Stats struct {
	Config
	sync.RWMutex
	workers    map[string]workerEntry
	lastReport time.Time
}

type Config struct {
	AbsoluteGoodThroughput float64
}

type Sample struct {
	Bytes    int64
	Duration time.Duration
}

func (s Sample) String() string {
	return fmt.Sprintf("%.2f MiB/s", s.Throughput()/1024/1024)
}

func (s Sample) Throughput() float64 {
	return float64(s.Bytes) / s.Duration.Seconds()
}

type workerEntry struct {
	samples int
	average float64
}

type namedEntry struct {
	name       string
	throughput float64
}

func New(c Config) *Stats {
	return &Stats{
		Config:  c,
		workers: map[string]workerEntry{},
	}
}

func (s *Stats) Remove(name string) {
	s.Lock()
	defer s.Unlock()

	delete(s.workers, name)
}

func (s *Stats) Update(name string, sample Sample) {
	if sample.Bytes < minSampleBytes && sample.Duration < minDurationForMinBytes {
		log.Debugf("Not enough bytes for a meaningful throughput sample, dropping (%d)", sample.Bytes)
		return
	}

	if sample.Duration < minSampleDuration {
		log.Debugf("Skipping stats recording for short duration %v", sample.Duration)
		return
	}

	log.Infof("Recording sample of %s for %s", sample.String(), name)

	defer func() {
		go s.report()
	}()

	s.Lock()
	defer s.Unlock()

	w := s.workers[name]
	w.average = (w.average*float64(w.samples) + sample.Throughput()) / (float64(w.samples) + 1)
	w.samples++
	if w.samples > maxSamples {
		// As time passes, mirrors that performed very well in the past might stack an indefinitely large amount
		// of samples, which might bias how the mirror is performing now. To avoid this, the number of samples taken
		// into account is capped at maxSamples.
		w.samples = maxSamples
	}

	s.workers[name] = w
}

func (s *Stats) GoodPerformer(name string) bool {
	entries := s.workerList()

	if len(entries) < 3 {
		log.Debugf("Stats collected for less than 2 workers, cannot emit a judgement yet")
		return true
	}

	position := slices.IndexFunc(entries, func(entry namedEntry) bool {
		return entry.name == name
	})

	if position == -1 {
		log.Debugf("Worker %s is not ranked yet", name)
		return true
	}

	log.Debugf("Worker %s is in position %d/%d", name, position+1, len(s.workers))

	if entries[position].throughput > s.AbsoluteGoodThroughput {
		log.Debugf("Worker %s has an absolutely good throughput", name)
		return true
	}

	// We're good performers if we're earlier than the last two positions
	return position <= len(entries)-3
}

func (s *Stats) report() {
	if !s.shouldReport() {
		return
	}

	list := s.workerList()
	for position, worker := range list {
		log.Infof("Worker #%d (%s): %.2f MiB/s", position+1, worker.name, worker.throughput/1024/1024)
	}
}

func (s *Stats) shouldReport() bool {
	s.Lock()
	defer s.Unlock()

	if time.Since(s.lastReport) < 10*time.Second {
		return false
	}

	s.lastReport = time.Now()
	return true
}

func (s *Stats) workerList() []namedEntry {
	s.RLock()
	defer s.RUnlock()

	entries := make([]namedEntry, 0, len(s.workers))

	for wName, entry := range s.workers {
		if entry.average == 0 {
			continue
		}

		entries = append(entries, namedEntry{
			name:       wName,
			throughput: entry.average,
		})
	}

	slices.SortFunc(entries, func(a, b namedEntry) bool {
		// Less func is inverted to sort in descending order (from best to worst throughput)
		return a.throughput > b.throughput
	})

	return entries
}
