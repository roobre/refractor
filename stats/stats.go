package stats

import (
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"sync"
	"time"
)

const (
	minSampleBytes         = 1024
	minSampleDuration      = 200 * time.Millisecond
	absoluteGoodThroughput = 20.0 * 1024 * 1024
	maxSamples             = 20.0
)

type Stats struct {
	sync.RWMutex
	workers    map[string]workerEntry
	lastReport time.Time
}

type Sample struct {
	Bytes    int64
	Duration time.Duration
}

type workerEntry struct {
	samples int
	average float64
}

type namedEntry struct {
	name       string
	throughput float64
}

func New() *Stats {
	return &Stats{
		workers: map[string]workerEntry{},
	}
}

func (p *Stats) Remove(name string) {
	p.Lock()
	defer p.Unlock()

	delete(p.workers, name)
}

func (p *Stats) Update(name string, sample Sample) {
	if sample.Bytes < minSampleBytes {
		log.Debugf("Not enough bytes for a meaningful throughput sample, dropping (%d)", sample.Bytes)
		return
	}

	if sample.Duration < minSampleDuration {
		log.Debugf("Skipping stats recording for short duration %v", sample.Duration)
		return
	}

	throughput := float64(sample.Bytes) / sample.Duration.Seconds()
	log.Infof("Recording sample of %.2fMiB/s for %s", throughput/1024/1024, name)

	defer func() {
		go p.report()
	}()

	p.Lock()
	defer p.Unlock()

	w := p.workers[name]
	w.average = (w.average*float64(w.samples) + throughput) / (float64(w.samples) + 1)
	w.samples++
	if w.samples > maxSamples {
		// As time passes, mirrors that performed very well in the past might stack an indefinitely large amount
		// of samples, which might bias how the mirror is performing now. To avoid this, the number of samples taken
		// into account is capped at maxSamples.
		w.samples = maxSamples
	}

	p.workers[name] = w
}

func (p *Stats) GoodPerformer(name string) bool {
	entries := p.workerList()

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

	log.Debugf("Worker %s is in position %d/%d", name, position+1, len(p.workers))

	if entries[position].throughput > absoluteGoodThroughput {
		log.Debugf("Worker %s has an absolutely good throughput", name)
		return true
	}

	// We're good performers if we're earlier than the last two positions
	return position <= len(entries)-3
}

func (p *Stats) report() {
	if !p.shouldReport() {
		return
	}

	list := p.workerList()
	for position, worker := range list {
		log.Infof("Worker #%d (%s): %.2f MiB/s", position+1, worker.name, worker.throughput/1024/1024)
	}
}

func (p *Stats) shouldReport() bool {
	p.Lock()
	defer p.Unlock()

	if time.Since(p.lastReport) < 10*time.Second {
		return false
	}

	p.lastReport = time.Now()
	return true
}

func (p *Stats) workerList() []namedEntry {
	p.RLock()
	defer p.RUnlock()

	entries := make([]namedEntry, 0, len(p.workers))

	for wName, entry := range p.workers {
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
