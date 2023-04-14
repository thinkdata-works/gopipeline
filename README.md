## gopipeline

This is a zero-dependency library that allows for the setup and execution of concurrent multi-step pipelines. The pipeline may operate over a single input stream, or pipelines running over multiple streams can be coordinated to run concurrently.

## Installation

**Note**: gopipeline is developed and tested against Go 1.18, since it uses generic support.

```
$ go get github.com/thinkdata-works/gopipeline
```

## Quickstart

See `pipeline_test.go` for working example of implementation.

## Running tests

```
$ go test -v ./pkg/gopipeline
```

## License

`gopipeline` is licensed under the Apache 2.0 License, found in the LICENSE file.