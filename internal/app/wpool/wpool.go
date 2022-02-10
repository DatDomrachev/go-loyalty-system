package wpool

import (
	"context"
	"fmt"
	"sync"
)

func worker(ctx context.Context, jobs <-chan Job, results chan<- Result) {
	for {
		select {
		case job, ok := <-jobs:
			if !ok {
				return
			}
			results <- job.execute(ctx)
		case <-ctx.Done():
			fmt.Printf("cancelled worker. Error detail: %v\n", ctx.Err())
			results <- Result{
				Err: ctx.Err(),
			}
			return
		}
	}
}

type WorkerPool struct {
	workersCount int
	jobs         chan Job
	results      chan Result
	done 		 chan bool	
}

type WorkerPooler interface {
	Run(ctx context.Context) 
	GenerateFrom(jobsBulk Job)
	Results() <-chan Result
	BroadcastDone(flag bool)
}

func New(wcount int) (*WorkerPool) {
	
	workerPool := &WorkerPool{
		workersCount: wcount,
		jobs:         make(chan Job, wcount),
		results:      make(chan Result, wcount),
		done:		  make(chan bool),
	}

	return workerPool
}

func (wp *WorkerPool) Run(ctx context.Context) {
	var wg sync.WaitGroup

	for i := 0; i < wp.workersCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(ctx, wp.jobs, wp.results)
		}()
	}

	wg.Wait()
	close(wp.done)
	close(wp.results)
}


func (wp *WorkerPool) GenerateFrom(jobBulk Job) {
		wp.jobs <- jobBulk
}

func (wp *WorkerPool) Results() <-chan Result {
	return wp.results
}

func (wp *WorkerPool) Done() <- chan bool  {
	return wp.done
}

func (wp *WorkerPool) BroadcastDone(flag bool) {
	wp.done <- flag
}
