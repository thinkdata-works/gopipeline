![CI](https://github.com/thinkdata-works/gopipeline/actions/workflows/ci.yml/badge.svg)

## gopipeline

gopipeline provides a framework for running multi-step pipelines over a stream of items that need processing. It was primarly designed for cases where pipeline steps may take an non-trivial amount time to complete, since they may be doing something like waiting for disk IO or making some external request. The pipeline can be configured to run multiple replicas over the input stream to maximize taking advantage of compute resources.

The pipeline can also be configured to be used as part of other async processes

## Requirements

gopipeline requires Go >= 1.18 since it relies on generic support

## Installation

```
$ go get github.com/thinkdata-works/gopipeline
```

## Features and configuration

- Dynamic step definition - supply one or more pure go functions to define your pipeline work
- Custom error handler - register a pure go function to handle errors and toggle between halting and non-halting behaviours
- Wait group handling - Register the pipeline with one or more other waitgroups to build this into other asynchronous processes

## Quickstart

This fictional example uses a local resource which interacts with some external services

```go
package main

import (
	"context"

	"github.com/thinkdata-works/gopipeline/pkg/gopipeline"
)

type resource struct {
	id           string
	signature    string
	external_url string
}

func process1(ctx context.Context, resources []resource) []error {
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

	err := pipeline.Work(ctx)
	if err != nil {
		errs = append(errs, err)
	}

	return errs
}

func getNewExternalUrl(ctx context.Context, r *resource) (*resource, error) {
	// Dispatch external request
	r.external_url = external_services.GetNewUrl(r.id)
	return r, nil
}

func reSignResource(ctx context.Context, r *resource) (*resource, error) {
	// Dispatch request to create new signature
	r.signature = external_services.SignUrl(r.id, r.external_url)
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
