package gpool

import (
	"context"
	"fmt"
	"runtime"
	"sync"
)

// Pool is a pool of workers that process jobs from a set of process processors
type Pool struct {
	// processors is the list of process processors that each worker will listen to
	processors []IProcessor
	mu         sync.RWMutex
	// workers is the list of workers
	workers []*worker
}

// Stats represents the current state of the pool
type Stats struct {
	BusyCnt int
	IdleCnt int
	Total   int
}

func (s Stats) String() string {
	return fmt.Sprintf("Total: %d, Idle: %d, Busy: %d", s.Total, s.IdleCnt, s.BusyCnt)
}

func (p *Pool) Stats() *Stats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ws := &Stats{
		Total: len(p.workers),
	}

	for _, worker := range p.workers {
		if worker.isIdle() {
			ws.IdleCnt += 1
		} else {
			ws.BusyCnt += 1
		}
	}

	return ws
}

func (p *Pool) addWorker(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// create a new worker
	w := newWorker(ctx, p.processors)
	w.addProcessors(p.processors)
	p.workers = append(p.workers, w)
	// start the worker loop
	go w.loop()
}

func (p *Pool) Stop() {
	for _, w := range p.workers {
		w.stop()
	}
}

func (p *Pool) AddProcessors(processor IProcessor, processors ...IProcessor) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, w := range p.workers {
		w.addProcessor(processor)
		for _, p := range processors {
			w.addProcessor(p)
		}
	}
}

// NewPool creates a new pool of workers that process tasks from a set of processors
func NewPool(ctx context.Context, workers int, processors ...IProcessor) *Pool {
	if workers < 1 {
		workers = runtime.NumCPU()
	}

	p := &Pool{
		processors: processors,
	}

	// add initial workers
	for i := 0; i < workers; i++ {
		p.addWorker(ctx)
	}

	return p
}
