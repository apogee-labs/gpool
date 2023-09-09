package gpool

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestNewWorkerPool(t *testing.T) {
	numJobs := 1000

	handlerA := func(ctx context.Context, i int) (int, error) {
		time.Sleep(10 * time.Millisecond)
		return i * 2, nil
	}
	handlerB := func(ctx context.Context, s string) (string, error) {
		time.Sleep(10 * time.Millisecond)
		return "Hello " + s, nil
	}
	procA := NewProcessor[int, int](handlerA)
	procB := NewBufferedProcessor[string, string](1, handlerB)
	pool := NewPool(context.Background(), 100, procA, procB)
	aTasks := make([]*Task[int, int], numJobs)
	bTasks := make([]*Task[string, string], numJobs)
	for i := 0; i < numJobs; i++ {
		aTasks[i] = procA.Submit(i)
		bTasks[i] = procB.Submit(fmt.Sprintf("World %d", i))
	}
	for i := 0; i < numJobs; i++ {
		outA, _ := aTasks[i].Await().Unwrap()
		if outA != i*2 {
			t.Errorf("Expected %d, got %d", i*2, outA)
		}
		outB, _ := bTasks[i].Await().Unwrap()
		if outB != fmt.Sprintf("Hello World %d", i) {
			t.Errorf("Expected %s, got %s", fmt.Sprintf("Hello World %d", i), outB)
		}
	}
	// add a new processor
	procC := NewProcessor[int, int](func(ctx context.Context, i int) (int, error) {
		time.Sleep(10 * time.Millisecond)
		return i * 3, nil
	})
	pool.AddProcessors(procC)
	cTasks := make([]*Task[int, int], numJobs)
	for i := 0; i < numJobs; i++ {
		cTasks[i] = procC.Submit(i)
	}
	TaskWaitAll(cTasks)
	for i, task := range cTasks {
		out, _ := task.Result().Unwrap()
		fmt.Println(out)
		if out != i*3 {
			t.Errorf("Expected %d, got %d", i*3, out)
		}
	}

	for i := 0; i < numJobs; i++ {
		out, _ := cTasks[i].Await().Unwrap()
		fmt.Println(out)
		if out != i*3 {
			t.Errorf("Expected %d, got %d", i*3, out)
		}
	}
	pool.Stop()
}
