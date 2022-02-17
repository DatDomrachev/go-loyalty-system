package wpool

import (
	"context"
	"log"
)

type JobID string
type jobType string
type jobMetadata map[string]interface{}

type ExecutionFn func(ctx context.Context, args interface{}) (interface{}, error)

type JobDescriptor struct {
	ID       JobID
	JType    jobType
	Metadata map[string]interface{}
}

type Result struct {
	Value      interface{}
	Err        error
	Descriptor JobDescriptor
}

type Job struct {
	Descriptor JobDescriptor
	ExecFn     ExecutionFn
	Args       interface{}
}

func (j Job) execute(ctx context.Context) Result {
	value, err := j.ExecFn(ctx, j.Args)
	if err != nil {
		log.Printf("error into job %v : %v", j.Descriptor.ID, err.Error())
		return Result{
			Err:        err,
			Descriptor: j.Descriptor,
		}
	}
	log.Printf("executed %v", j.Descriptor.ID)
	return Result{
		Value:      value,
		Descriptor: j.Descriptor,
	}
}