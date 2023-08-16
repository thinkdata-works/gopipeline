![CI](https://github.com/thinkdata-works/gopipeline/actions/workflows/ci.yml/badge.svg)

## gopipeline

gopipeline provides a framework for running multi-step pipelines over a stream of items that need asynchronous processing. It was designed to allow pipeline steps to be long-running, since they may be I/O bound. The pipeline itself is configurable to run multiple copies to take advantage of compute resources.

The pipeline can also be configured to be used as part of other async processes by registering it as a waitgroup member.

## Requirements

gopipeline requires Go >= 1.18 since it relies on generic support

## Installation

```
$ go get github.com/thinkdata-works/gopipeline
```

## Features and configuration

- Dynamic step definition - supply one or more pure go functions to define your pipeline work
- Custom error handler - register a function to handle errors and toggle between halting and non-halting behaviours
- Wait group handling - Register the pipeline with one or more other waitgroups to build this into other asynchronous processes
- Stats reporting - asynchronously track the process of the pipeline for progress or benchmarking

## Quickstart

This fictional example uses a local resource which interacts with some external services

```go
package main

import (
	"context"
	"time"
	"github.com/thinkdata-works/gopipeline/pkg/gopipeline"
)

type resource struct {
	id           string
	signature    string
	external_url string
}

func process(ctx context.Context, resources []resource) []error {
	errs := []error{}

	// Define the new pipeline with concurrency count and size
	pipeline := gopipeline.NewPipeline[*resource](5, 100)

	// Register our function that will feed values to the top of the pipeline
	pipeline.RegisterInputProvider(func(ctx context.Context, c chan *resource) {
		defer close(c)
		for _, r := range resources {
			c <- &r
		}
	})

	// Register our error handler
	pipeline.RegisterErrorHandler(func(err error) bool {
		errs = append(errs, err)
		return true // return false for non-halting
	})

	// Compose our steps
	pipeline.RegisterSteps(
		getNewExternalUrl, reSignResource, applyChanges, notifyDownstream,
	)

	pipeline.RegisterReporter(
		5 * time.Second, func(r Report) {
			fmt.Printf(
			"\n\n=====Begin stats=====\nTotal items finished: %d\nTotal items in pipeline: %d\nAverage items per second: %.6f\n=====End stats=====\n\n",
			r.TotalFinished, r.TotalInPipeline, r.ItemsPerSecond,
			)
		},
	)

	err := pipeline.Work(ctx)
	if err != nil {
		errs = append(errs, err)
	}

	return errs
}

func getNewExternalUrl(ctx context.Context, r *resource) (*resource, error) {
	// Dispatch external request
	url, err := external_services.GetNewUrl(r.id)
	if err != nil {
		return r, err
	}
	r.external_url = url
	return r, nil
}

func reSignResource(ctx context.Context, r *resource) (*resource, error) {
	// Dispatch request to create new signature
	signature, err := external_services.SignUrl(r.id, r.external_url)
	if err != nil {
		return r, err
	}
	r.signature = signature
	return r, nil
}

func applyChanges(ctx context.Context, r *resource) (*resource, error) {
	// Apply changes to some kind of storage
	err := storage.ApplyResourceChanges(r.id, r.signature, r.external_url)
	if err != nil {
		return r, err
	}
	return r, nil
}

func notifyDownstream(ctx context.Context, r *resource) (*resource, error) {
	external_services.NotifyListeners(r.id, r.signature, r.external_url)
	return r, nil
}

```

Also see `pipeline_test.go` for additional working examples, as well as examples for running multiple pipelines concurrently.

## Running tests

```
$ go test -v ./pkg/gopipeline
```

## License

`gopipeline` is licensed under the Apache 2.0 License, found in the LICENSE file.

## Special thanks

To [Matt Pollack](https://github.com/mattpollack) and [Kevin Birk](https://github.com/kbirk) for their design and authorship of this project.
