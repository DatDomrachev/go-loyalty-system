package wpool

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"
	"errors"
	"log"
)

const (
	jobsCount   = 10
	workerCount = 2
)

func TestWorkerPool(t *testing.T) {
	wp := New(workerCount)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	go wp.Run(ctx)

	jobs:= testJobs()

	
	for i := range jobs {
		go wp.GenerateFrom(jobs[i])
	}
	
	results := 0;

	for {
		select {
		case r, ok := <-wp.Results():
			if !ok {
				continue
			}

			i, err := strconv.ParseInt(string(r.Descriptor.ID), 10, 64)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			val := r.Value.(int)
			if val != int(i)*2 {
				t.Fatalf("wrong value %v; expected %v", val, int(i)*2)
			} else {
				log.Printf("result: %v", val)
				results = results + 1
				if results == jobsCount {
					go wp.BroadcastDone(true)
				}
			}
		case <-wp.Done():
			log.Print("done")
			return
		default:
			log.Print("waiting")
		}
	}
}

func TestWorkerPool_TimeOut(t *testing.T) {
	wp := New(workerCount)

	ctx, cancel := context.WithTimeout(context.TODO(), time.Nanosecond*10)
	defer cancel()

	go wp.Run(ctx)

	for {
		select {
		case r := <-wp.Results():
			if r.Err != nil && r.Err != context.DeadlineExceeded {
				t.Fatalf("expected error: %v; got: %v", context.DeadlineExceeded, r.Err)
			}
		case <-wp.Done():
			return
		default:
			log.Print("waiting")
		}
	}
}

func TestWorkerPool_Cancel(t *testing.T) {
	wp := New(workerCount)

	ctx, cancel := context.WithCancel(context.TODO())

	go wp.Run(ctx)
	cancel()

	for {
		select {
		case r := <-wp.Results():
			if r.Err != nil && r.Err != context.Canceled {
				t.Fatalf("expected error: %v; got: %v", context.Canceled, r.Err)
			}
		case <-wp.Done():
			return
		default:
			log.Print("waiting")
		}
	}
}

func testJobs() []Job {
	execFn := func(ctx context.Context, args interface{}) (interface{}, error) {
		argVal, ok := args.(int)
		if !ok {
			return nil, errors.New("wrong argument type")
		}

		return argVal * 2, nil
	}

	jobs := make([]Job, jobsCount)
	for i := 0; i < jobsCount; i++ {
		jobs[i] = Job{
			Descriptor: JobDescriptor{
				ID:       JobID(fmt.Sprintf("%v", i)),
				JType:    "anyType",
				Metadata: nil,
			},
			ExecFn: execFn,
			Args:   i,
		}
	}
	return jobs
}