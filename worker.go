package gpool

import (
	"context"
	"sync"
	"sync/atomic"
)

type worker struct {
	ctx         context.Context
	processors  []IProcessor
	status      uint32
	cancel      context.CancelFunc
	processorWg sync.WaitGroup
	inputCh     chan iTaskWithHandler
}

func (w *worker) addProcessor(p IProcessor) {
	w.processors = append(w.processors, p)
	w.processorWg.Add(1)
	r := p.register()

	go func() {
		defer func() {
			w.processorWg.Done()
			r.Cancel()
		}()
		for {
			select {
			case <-w.ctx.Done():
				return
			case t := <-r.C():
				// emit the job to the worker input channel
				w.inputCh <- t
			}
		}
	}()
}

func (w *worker) addProcessors(ps []IProcessor) {
	for _, p := range ps {
		w.addProcessor(p)
	}
}

func (w *worker) loop() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case t := <-w.inputCh:
			atomic.SwapUint32(&w.status, 0)
			t.execute(w.ctx)
			atomic.SwapUint32(&w.status, 1)
		}
	}
}

func (w *worker) isIdle() bool {
	return atomic.LoadUint32(&w.status) == 0
}

func (w *worker) isBusy() bool {
	return atomic.LoadUint32(&w.status) == 1
}

func (w *worker) stop() {
	w.cancel()
	w.processorWg.Wait()
}

func newWorker(ctx context.Context, processors []IProcessor) *worker {
	wCtx, cancel := context.WithCancel(ctx)
	return &worker{
		processors: processors,
		ctx:        wCtx,
		cancel:     cancel,
		inputCh:    make(chan iTaskWithHandler, 1),
	}
}
