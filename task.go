package gpool

import (
	"context"
	"sync"
	"time"
)

type JobStatus uint32

const (
	TaskStatusPending JobStatus = iota
	TaskStatusRunning
	TaskStatusDone
)

// TaskMeta is the metadata for a taskWithHandler including timings
type TaskMeta struct {
	created  time.Time
	started  time.Time
	finished time.Time
}

// Created returns the time the taskWithHandler was created
func (s *TaskMeta) Created() time.Time {
	return s.created
}

// Started returns the time the taskWithHandler was started
func (s *TaskMeta) Started() time.Time {
	return s.started
}

// Finished returns the time the taskWithHandler was finished
func (s *TaskMeta) Finished() time.Time {
	return s.finished
}

// TotalDuration returns the total duration of the taskWithHandler
func (s *TaskMeta) TotalDuration() time.Duration {
	return s.finished.Sub(s.started)
}

// WaitDuration returns the duration the taskWithHandler was waiting to be executed
func (s *TaskMeta) WaitDuration() time.Duration {
	return s.started.Sub(s.created)
}

// ExecDuration returns the duration the taskWithHandler was executing
func (s *TaskMeta) ExecDuration() time.Duration {
	return s.finished.Sub(s.started)
}

// TaskResult is the result of a taskWithHandler
type TaskResult[O, I any] struct {
	// output is the output of the taskWithHandler
	output O
	// input is the input of the taskWithHandler
	input I
	// err will be set if the taskWithHandler failed
	err error
	// meta is the metadata for the taskWithHandler
	meta *TaskMeta
}

func (p *TaskResult[O, I]) Output() O {
	return p.output
}

func (p *TaskResult[O, I]) Input() I {
	return p.input
}

func (p *TaskResult[O, I]) Error() error {
	return p.err
}

// Unwrap returns the output and error of the taskWithHandler
func (p *TaskResult[O, I]) Unwrap() (O, error) {
	return p.output, p.err
}

type Awaitable interface {
	Done() <-chan struct{}
	StatusCh(status JobStatus) <-chan struct{}
}

// Task is a taskWithHandler that can be executed by a pool worker
// Task will be returned by a processor when a job is submitted
// once a `Task` is added to the pool a taskWithHandler can be awaited for completion
// by calling `Await()`
type Task[I, O any] struct {
	in     I
	result *TaskResult[O, I]
	meta   *TaskMeta
	mu     sync.RWMutex
	status JobStatus
}

// StatusCh returns a channel that will be closed when the taskWithHandler status is the given status
func (j *Task[I, O]) StatusCh(status JobStatus) <-chan struct{} {
	var done bool
	ch := make(chan struct{})
	go func() {
		for !done {
			done = j.Status() == status
		}
		ch <- struct{}{}
		close(ch)
	}()
	return ch
}

// Done returns a channel that will be closed when the task is Done
func (j *Task[I, O]) Done() <-chan struct{} {
	return j.StatusCh(TaskStatusDone)
}

// Meta returns the current metadata for the taskWithHandler
func (j *Task[I, O]) Meta() *TaskMeta {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.meta
}

// Await waits for the taskWithHandler to be completed and returns the result
func (j *Task[I, O]) Await() *TaskResult[O, I] {
	<-j.Done()
	return j.Result()
}

// Status returns the current status of the taskWithHandler
func (j *Task[I, O]) Status() JobStatus {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status
}

// Result returns the current result of the taskWithHandler if any
func (j *Task[I, O]) Result() *TaskResult[O, I] {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.result
}

func (j *Task[I, O]) started() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.meta.started = time.Now()
	j.status = TaskStatusRunning
}

// iTaskWithHandler is an interface for a taskWithHandler that can be executed by a pool worker
type iTaskWithHandler interface {
	execute(context.Context)
	Status() JobStatus
	Meta() *TaskMeta
}

// taskWithHandler is the typed task with a typed handler that can be executed by a pool worker
// this struct is passed to a worker's input channel and then executed by calling `execute()`
// we need to couple the task with the handler in the worker queue so that workers can
// work on multiple tasks with different types from different typed processors
// this way we avoid needing unnecessary type assertions in the worker loop
type taskWithHandler[I, O any] struct {
	// Task is the task parent that was returned by the processor on submission
	Task *Task[I, O]
	// the handler function to execute by a worker
	handler Handler[I, O]
	// once used to ensure that the taskWithHandler is only executed once
	once sync.Once
}

// execute executes the provided handler and sets the result
func (j *taskWithHandler[I, O]) execute(ctx context.Context) {
	// prevent race conditions in case multiple processors are passed the same job
	// by using a sync.Once
	j.once.Do(func() {
		j.Task.started()
		out, err := j.handler(ctx, j.Task.in)
		j.Task.meta.finished = time.Now()
		j.Task.status = TaskStatusDone
		j.Task.mu.Lock()
		j.Task.result = &TaskResult[O, I]{
			output: out,
			input:  j.Task.in,
			err:    err,
			meta:   j.Task.meta,
		}
		j.Task.mu.Unlock()
	})
}

// Status returns the current status of the taskWithHandler
func (j *taskWithHandler[I, O]) Status() JobStatus {
	return j.Task.Status()
}

// Meta returns the current metadata for the taskWithHandler
func (j *taskWithHandler[I, O]) Meta() *TaskMeta {
	return j.Task.Meta()
}

// newTaskWithHandler creates a new taskWithHandler with the given input and handler
func newTaskWithHandler[I, O any](in I, handler Handler[I, O]) *taskWithHandler[I, O] {
	return &taskWithHandler[I, O]{
		Task: &Task[I, O]{
			in:     in,
			status: TaskStatusPending,
			meta: &TaskMeta{
				created: time.Now(),
			},
		},
		handler: handler,
	}
}

func TaskAggregate[I, O any](tasks []*Task[I, O]) <-chan *TaskResult[O, I] {
	// create aggregate channel
	agg := make(chan *TaskResult[O, I])
	var wg sync.WaitGroup
	wg.Add(len(tasks))

	// loop over all tasks and await them
	for _, t := range tasks {
		go func(t *Task[I, O]) {
			defer wg.Done()
			agg <- t.Await()
		}(t)
	}

	// close channel when all tasks are done
	go func() {
		wg.Wait()
		close(agg)
	}()

	return agg
}

func TaskWaitAll[I, O any](tasks []*Task[I, O]) {
	for _, t := range tasks {
		<-t.Done()
	}
}
