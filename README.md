# gpool

gpool is a dead simple goroutine pool library with zero dependencies that supports
generics and `promise` like semantics for handling the return value of submitted tasks.

## Usage

```go
package main

import (
	"context"
	"fmt"
	gp "github.com/apogee-labs/gpool"
	"time"
)

func main() {
	// handlers will always accept a context as the first argument and return an error as the last argument
	// the context is passed from the worker goroutine and can be used
	// to stop the task when the pool is stopped
	handlerA := func(ctx context.Context, i int) (int, error) {
		return i * 2, nil
	}
	
	// create 2 processors, one for each handler
	processorA := gp.NewProcessor[int, int](handlerA)
	processorB := gp.NewBufferedProcessor[string, string](3, func(ctx context.Context, s string) (string, error) {
		return s + " World", nil
	})

	// create a pool with 10 workers and one or more processors
	pool := gp.NewPool(context.TODO(), 10, processorA)
	// processors can also be added after the pool is created with:
	pool.AddProcessors(processorB)
	
	numJobs := 100
	
	aTasks := make([]*gp.Task[int, int], numJobs)
	bTasks := make([]*gp.Task[int, int], numJobs)
	
	// submit jobs to the processors
	for i := 0; i < numJobs; i++ {
		aTasks[i] = processorA.Submit(i)
		bTasks[i] = processorB.Submit(fmt.Sprintf("%d says Hello", i))
    }

	// wait for all jobs to complete
	for i := 0; i < numJobs; i++ {
		// `Await()` waits for the Result to be returned which can be unwrapped to get the output with `Unwrap()`
		outA, _ := aTasks[i].Await().Unwrap()
		fmt.Println(outA)
		outB, _ := bTasks[i].Await().Unwrap()
		fmt.Println(outB)
	}
	
	pool.Stop()
}
```

## TODO
- [ ] exhaustive tests
- [ ] benchmarks
- [ ] autoscaling
- [ ] more examples
- [ ] test and coverage reporting
- [ ] commit hooks