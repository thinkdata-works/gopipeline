package gopipeline

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testItem struct {
	id       int
	intval   int
	strval   string
	strarray []string
}

func TestPipelineSingle(t *testing.T) {
	pipeline := NewPipeline[*testItem](1, 3)

	// create placeholder for test vals
	items := make([]testItem, 10)
	pipeline.RegisterInputProvider(testPipelineProvider(&items))
	pipeline.RegisterSteps(
		stepOne, stepTwo, stepThree,
	)
	err := pipeline.Work(context.Background())
	assert.NoError(t, err)

	assertItems(t, items)
}

func TestPipelineMultiple(t *testing.T) {
	pipeline := NewPipeline[*testItem](5, 20)

	// create placeholder for test vals
	items := make([]testItem, 1000)
	pipeline.RegisterInputProvider(testPipelineProvider(&items))
	pipeline.RegisterSteps(
		stepOne, stepTwo, stepThree,
	)
	err := pipeline.Work(context.Background())
	assert.NoError(t, err)

	assertItems(t, items)
}

func TestMultiplePipelines(t *testing.T) {
	pipeline1 := NewPipeline[*testItem](3, 10)
	pipeline2 := NewPipeline[*testItem](5, 25)

	items1 := make([]testItem, 4000)
	items2 := make([]testItem, 1600)

	pipeline1.RegisterInputProvider(testPipelineProvider(&items1))
	pipeline2.RegisterInputProvider(testPipelineProvider(&items2))

	pipeline1.RegisterSteps(
		stepOne, stepTwo, stepThree,
	)
	pipeline2.RegisterSteps(
		stepOne, stepTwo, stepThree,
	)

	wg := sync.WaitGroup{}
	pipeline1.RegisterWaitGroups(&wg)
	pipeline2.RegisterWaitGroups(&wg)

	go pipeline1.Work(context.Background())
	go pipeline2.Work(context.Background())

	wg.Wait()

	assertItems(t, items1)
	assertItems(t, items2)
}

func TestPipeline_NoInputProvider(t *testing.T) {
	pipeline := NewPipeline[*testItem](1, 10)
	pipeline.RegisterSteps(stepOne, stepTwo, stepThree)
	err := pipeline.Work(context.Background())
	assert.Errorf(t, err, "must register input provider")
}

func TestPipeline_NoSteps(t *testing.T) {
	pipeline := NewPipeline[*testItem](1, 10)
	items := make([]testItem, 1000)
	pipeline.RegisterInputProvider(testPipelineProvider(&items))
	err := pipeline.Work(context.Background())
	assert.Errorf(t, err, "must register at least one step")
}

func TestPipeline_ErrorHandler(t *testing.T) {
	pipeline := NewPipeline[*testItem](1, 3)

	errs := []error{}

	// create placeholder for test vals
	items := make([]testItem, 10)
	pipeline.RegisterInputProvider(testPipelineProvider(&items))
	pipeline.RegisterErrorHandler(func(err error) bool {
		errs = append(errs, err)
		return false //non-halting
	})
	pipeline.RegisterSteps(
		stepOne, stepTwoError, stepThree,
	)
	err := pipeline.Work(context.Background())
	assert.NoError(t, err)

	assertItemsWithStepTwoError(t, items)
}

func TestPipeline_ErrorHandler_Halting(t *testing.T) {
	pipeline := NewPipeline[*testItem](1, 3)

	errs := []error{}

	// create placeholder for test vals
	items := make([]testItem, 10)
	pipeline.RegisterInputProvider(testPipelineProvider(&items))
	pipeline.RegisterErrorHandler(func(err error) bool {
		errs = append(errs, err)
		return true // halting
	})
	pipeline.RegisterSteps(
		stepOne, stepTwoError, stepThree,
	)
	err := pipeline.Work(context.Background())
	assert.NoError(t, err)

	assertItemsWithStepTwoHaltingError(t, items)
}

func TestPipeline_Reporter(t *testing.T) {
	pipeline := NewPipeline[*testItem](5, 20)

	// create placeholder for test vals
	items := make([]testItem, 1000)
	pipeline.RegisterInputProvider(testPipelineProvider(&items))
	pipeline.RegisterSteps(
		stepOne, stepTwo, func(ctx context.Context, ti *testItem) (*testItem, error) {
			time.Sleep(5 * time.Millisecond)
			return ti, nil
		}, stepThree,
	)

	var reportCalled int
	previousTotal := int64(0)
	pipeline.RegisterReporter(50*time.Millisecond, func(r Report) {
		reportCalled++

		// Numbers should be increasing
		assert.Greater(t, r.TotalFinished, previousTotal)
		previousTotal = r.TotalFinished

		// Set minimum
		assert.Greater(t, r.ItemsPerSecond, float64(50))

		// No sensible assertions for TotalInPipeline
	})
	err := pipeline.Work(context.Background())
	assert.NoError(t, err)

	assertItems(t, items)
	assert.GreaterOrEqual(t, reportCalled, 20)
}

func assertItems(t *testing.T, items []testItem) {
	for i, item := range items {
		assert.Equal(t, i, item.id)
		assert.Equal(t, i*2*2, item.intval)
		assert.Equal(t, fmt.Sprintf("Step two for %d", item.id), item.strval)
		assert.Len(t, item.strarray, 3)
		assert.Equal(t, fmt.Sprintf("%d", item.id), item.strarray[0])
		assert.Equal(t, item.strval, item.strarray[1])
		assert.Equal(t, "I am done", item.strarray[2])
	}
}

func assertItemsWithStepTwoError(t *testing.T, items []testItem) {
	for i, item := range items {
		assert.Equal(t, i, item.id)
		assert.Equal(t, i*2*2, item.intval)
		assert.Equal(t, "", item.strval)
		assert.Len(t, item.strarray, 3)
		assert.Equal(t, fmt.Sprintf("%d", item.id), item.strarray[0])
		assert.Equal(t, "", item.strarray[1])
		assert.Equal(t, "I am done", item.strarray[2])
	}
}

func assertItemsWithStepTwoHaltingError(t *testing.T, items []testItem) {
	for i, item := range items {
		assert.Equal(t, i, item.id)
		assert.Equal(t, i*2*2, item.intval)
		assert.Equal(t, "", item.strval)
		assert.Len(t, item.strarray, 0)
	}
}

func stepOne(ctx context.Context, item *testItem) (*testItem, error) {
	// Increment intval by id * 2 * 2
	item.intval = item.id * 2 * 2
	return item, nil
}

func stepTwo(ctx context.Context, item *testItem) (*testItem, error) {
	item.strval = fmt.Sprintf("Step two for %d", item.id)
	return item, nil
}

func stepTwoError(ctx context.Context, item *testItem) (*testItem, error) {
	return item, fmt.Errorf("no can do pal!")
}

func stepThree(ctx context.Context, item *testItem) (*testItem, error) {
	item.strarray = append(item.strarray, fmt.Sprintf("%d", item.id))
	item.strarray = append(item.strarray, item.strval)
	item.strarray = append(item.strarray, "I am done")
	return item, nil
}

// Return a function that generates a bunch of items
func testPipelineProvider(items *[]testItem) func(context.Context, chan *testItem) {
	return func(ctx context.Context, c chan *testItem) {
		defer close(c)

		for i := range *items {
			(*items)[i] = testItem{
				id:       i,
				intval:   0,
				strval:   "",
				strarray: []string{},
			}
			c <- &(*items)[i]
		}
	}
}
