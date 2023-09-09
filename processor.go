package gpool

import (
	"context"
	"sync/atomic"
)

// IProcessor is an interface for a processor
type IProcessor interface {
	register() *registration
}

// Handler is a function that processes a job for a processor
type Handler[I, O any] func(context.Context, I) (O, error)

type registration struct {
	c      <-chan iTaskWithHandler
	cancel context.CancelFunc
}

func (r *registration) Cancel() {
	r.cancel()
}

func (r *registration) C() <-chan iTaskWithHandler {
	return r.c
}

// Processor is a queue that processes jobs with the given handler
type Processor[I, O any] struct {
	queue      chan *taskWithHandler[I, O]
	handler    Handler[I, O]
	registered uint64
}

// Submit submits a taskWithHandler to the queue with the given input
func (p *Processor[I, O]) Submit(input I) *Task[I, O] {
	if !p.isRegistered() {
		panic("cannot submit a task to an unregistered processor")
	}

	t := newTaskWithHandler[I, O](input, p.handler)
	p.queue <- t
	// return the base task without the handler
	return t.Task
}

// SubmitBatch submits a batch of tasks to the queue with the given input
func (p *Processor[I, O]) SubmitBatch(input []I) []*Task[I, O] {
	tasks := make([]*Task[I, O], len(input))
	for i, in := range input {
		tasks[i] = p.Submit(in)
	}
	return tasks
}

// TrySubmit attempts to submit a task, but if the queue is full it will return nil
func (p *Processor[I, O]) TrySubmit(input I) *Task[I, O] {
	if !p.isRegistered() {
		panic("cannot submit a task to an unregistered processor")
	}

	t := newTaskWithHandler[I, O](input, p.handler)
	select {
	case p.queue <- t:
		return t.Task
	default:
		return nil
	}
}

func (p *Processor[I, O]) isRegistered() bool {
	return atomic.LoadUint64(&p.registered) > 0
}

// register returns a channel that emits jobs from the queue
func (p *Processor[I, O]) register() *registration {
	ch := make(chan iTaskWithHandler)
	ctx, cancel := context.WithCancel(context.Background())

	atomic.AddUint64(&p.registered, 1)

	// goroutine that emits jobs from the subscription and pushes them
	// to the generic taskWithHandler channel
	go func() {
		defer func() {
			close(ch)
			atomic.AddUint64(&p.registered, ^uint64(0))
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-p.queue:
				ch <- job
			}
		}
	}()

	return &registration{
		c:      ch,
		cancel: cancel,
	}
}

// NewProcessor creates a new processor with the given handler
func NewProcessor[I, O any](handler Handler[I, O]) *Processor[I, O] {
	return &Processor[I, O]{
		queue:   make(chan *taskWithHandler[I, O]),
		handler: handler,
	}
}

// NewBufferedProcessor creates a new processor with the given buffer size and handler
func NewBufferedProcessor[I, O any](bufferSize int, handler Handler[I, O]) *Processor[I, O] {
	return &Processor[I, O]{
		queue:   make(chan *taskWithHandler[I, O], bufferSize),
		handler: handler,
	}
}
