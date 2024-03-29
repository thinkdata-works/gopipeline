package gopipeline

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Pipeline[I any] interface {
	/*
		Register one or more pipeline steps. These functions should take in an item, and do any kind of work on it. They should return the transformed version of the item

		The steps will be run in order, and scaled out for each item. It would be best if the items being processed did not share state

		If the step returns an error, then the pipeline will stop processing the current item. The step itself is responsible for any reporting of the error
	*/
	RegisterSteps(...func(context.Context, I) (I, error))

	/*
		Register a function that will send items to the inputStream
		**This function is responsible for closing the steam when it is finished**

		Subsequent calls will replace the input handler
	*/
	RegisterInputProvider(func(ctx context.Context, inputStream chan I))

	/*
		Register a function that will receive errors returned from any of the pipeline steps.

		If the handler returns `true` then execution of the next pipeline step will continue. If it returns false, or if the error handler is not defined, then pipeline will cease execution for the current item.

		Invoking the error handler users a mutex, so any action it takes will be threadsafe.

		Subsequent calls with replace the error handler
	*/
	RegisterErrorHandler(func(error) bool)

	/*
		Register one or more waitgroups. After `Work()` returns, it will call `.Done()` on each waitgroup on order, whether it completes execution or not. This will allow the pipeline to be called asynchronously

		At registration, `wg.Add(1)` will be called for each group given
	*/
	RegisterWaitGroups(...*sync.WaitGroup)

	/*
		Register a time interival in which to run a stats reporter on the pipeline. Also register a function which will recieve the stats report. You may use this function for printing status of the pipeline or benchmarking

		Subsequent calls will replace the reporter
	*/
	RegisterReporter(reportInterval time.Duration, reporter func(r Report))

	/*
		Begins pipeline execution. Returns errors if they occur during setup

		PLEASE NOTE that this function is not idempotent. Running the work function twice will not produce the same result. For subsequent runs, please create a new pipeline
	*/
	Work(ctx context.Context) error
}

type pipeline[I any] struct {
	concurrencyLevel int
	bufferSize       int

	// Pipeline steps operate on an item, and return some possibly transformed item
	steps         []func(context.Context, I) (I, error)
	inputStream   chan I
	inputProvider func(context.Context, chan I)
	errorHandler  func(error) bool
	errorMtx      *sync.Mutex
	listeningwgs  []*sync.WaitGroup

	// internals
	pipelineWg *sync.WaitGroup

	// stats
	stats *stats
}

func NewPipeline[I any](concurrencyLevel, bufferSize int) Pipeline[I] {
	p := &pipeline[I]{
		concurrencyLevel: concurrencyLevel,
		bufferSize:       bufferSize,
		pipelineWg:       &sync.WaitGroup{},
		errorMtx:         &sync.Mutex{},
		listeningwgs:     []*sync.WaitGroup{},
	}
	return p
}

func (p *pipeline[I]) RegisterSteps(steps ...func(context.Context, I) (I, error)) {
	p.steps = append(p.steps, steps...)
}

func (p *pipeline[I]) RegisterInputProvider(provider func(context.Context, chan I)) {
	p.inputProvider = provider
}

func (p *pipeline[I]) RegisterErrorHandler(handler func(error) bool) {
	p.errorHandler = handler
}

func (p *pipeline[I]) RegisterWaitGroups(groups ...*sync.WaitGroup) {
	for _, g := range groups {
		g.Add(1)
		p.listeningwgs = append(p.listeningwgs, g)
	}
}

func (p *pipeline[I]) RegisterReporter(reportInterval time.Duration, reporter func(r Report)) {
	p.stats = newStatsTracker(reportInterval, reporter)
}

func (p *pipeline[I]) Work(ctx context.Context) error {
	if p.stats != nil {
		p.stats.start(time.Now())
		defer func() {
			p.stats.done()
		}()
	}

	defer func() {
		for _, g := range p.listeningwgs {
			g.Done()
		}
	}()

	// Throw an error if no input provider is given
	if p.inputProvider == nil {
		return fmt.Errorf("must register input provider")
	}

	if len(p.steps) < 1 {
		return fmt.Errorf("must register at least one step")
	}

	if p.stats != nil {
		// Append a step to the end where the item is completed
		p.steps = append(p.steps, func(ctx context.Context, i I) (I, error) {
			p.stats.registerItemComplete()
			return i, nil
		})
	}

	// Create the channel that inputs will be passed to
	p.inputStream = make(chan I, p.bufferSize)

	// Create the concurrent pipelines according to the given value
	for w := 1; w <= p.concurrencyLevel; w++ {
		// Create an executor for the pipeline, with the step of steps that it will do
		e := newExecutor(p)
		e.setup(ctx)
		go e.work(ctx)
	}

	p.inputProvider(ctx, p.inputStream)
	p.pipelineWg.Wait()
	return nil
}

type executor[I any] struct {
	pipeline   *pipeline[I]
	topChannel chan I
}

func newExecutor[I any](pipeline *pipeline[I]) *executor[I] {
	return &executor[I]{pipeline: pipeline}
}

func (e *executor[I]) setup(ctx context.Context) {
	// Create two channels holders that will be our upstream and downstream channels
	var upstreamChan, downstreamChan chan I
	for i, step := range e.pipeline.steps {
		// For each step added, increment the waitgroup for the whole pipeline
		e.pipeline.pipelineWg.Add(1)

		// For each step, we want to set an upstream channel (to receive items) and a downstream channel (to send items to)
		if upstreamChan == nil {
			// If the upstream channel is undefined, then begin setup
			upstreamChan = make(chan I, e.pipeline.bufferSize)
			// And store this as the top of our executor
			e.topChannel = upstreamChan
		} else {
			// Otherwise, upstreamChan should be the previous step's downstream
			upstreamChan = downstreamChan
		}

		// if we are at the last step
		if i+1 == len(e.pipeline.steps) {
			// downstream chan should be set to nil
			downstreamChan = nil
		} else {
			// create a new downstream channel
			downstreamChan = make(chan I, e.pipeline.bufferSize)
		}

		// Run the step with our channels
		go e.runStep(ctx, upstreamChan, downstreamChan, step)
	}
}

func (e *executor[I]) runStep(ctx context.Context, upstream, downstream chan I, step func(context.Context, I) (I, error)) {
	defer e.pipeline.pipelineWg.Done()

	// Close downstream channel at end of execution if it exists
	if downstream != nil {
		defer close(downstream)
	}

	for item := range upstream {
		transformedItem, err := step(ctx, item)

		if err != nil {
			var shouldcontinue bool
			if e.pipeline.errorHandler != nil {
				shouldcontinue = e.handleError(err)
			}

			if shouldcontinue {
				continue
			}
		}

		// continue pipeline if defined
		if downstream != nil {
			downstream <- transformedItem
		}
	}
}

func (e *executor[I]) handleError(err error) bool {
	e.pipeline.errorMtx.Lock()
	defer e.pipeline.errorMtx.Unlock()
	return e.pipeline.errorHandler(err)
}

// Kick off the executor by sending items from the input stream
func (e *executor[I]) work(ctx context.Context) {
	defer close(e.topChannel)

	for item := range e.pipeline.inputStream {
		// If we are collecting stats for the pipeline then register each new item
		if e.pipeline.stats != nil {
			e.pipeline.stats.registerNewItem()
		}
		e.topChannel <- item
	}
}
