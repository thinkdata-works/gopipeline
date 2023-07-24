package gopipeline

import (
	"sync"
	"time"
)

type stats struct {
	mutex     *sync.RWMutex
	startedAt *time.Time

	ticker   *time.Ticker
	doneChan chan bool

	// Current number of items being processed by the pipeline
	totalInPipeline int64
	// Total items that have finished processing by the pipeline
	totalFinished int64
	// Number of items processed per second by the pipeline
	itemsPerSecond float64

	// Function to report the stats
	reporter func(r Report)
	interval time.Duration
}

type Report struct {
	// Current number of items being processed by the pipeline
	TotalInPipeline int64
	// Total items that have finished processing by the pipeline
	TotalFinished int64
	// Number of items processed per second by the pipeline
	ItemsPerSecond float64
}

func newStatsTracker(interval time.Duration, reporter func(r Report)) *stats {
	s := &stats{
		mutex:    &sync.RWMutex{},
		interval: interval,
		reporter: reporter,
	}
	return s
}

func (s *stats) start(t time.Time) {
	s.startedAt = &t

	s.ticker = time.NewTicker(s.interval)
	s.doneChan = make(chan bool)

	go func() {
		for {
			select {
			case <-s.doneChan:
				return
			case <-s.ticker.C:
				s.report()
			}
		}
	}()
}

func (s *stats) registerNewItem() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.totalInPipeline++
}

func (s *stats) registerItemComplete() {
	s.mutex.Lock()
	s.totalFinished++
	s.totalInPipeline--
	s.mutex.Unlock()

	s.itemsPerSecond = float64(s.totalFinished) / time.Since(*s.startedAt).Seconds()
}

func (s *stats) done() {
	s.ticker.Stop()
	s.doneChan <- true
	s.report() // one last report
}

func (s *stats) report() {
	// Ext if there is nothing to report
	if s.reporter == nil {
		return
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()
	s.reporter(Report{
		ItemsPerSecond:  s.itemsPerSecond,
		TotalFinished:   s.totalFinished,
		TotalInPipeline: s.totalInPipeline,
	})
}
