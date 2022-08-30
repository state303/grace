package grace

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestTask_NextMustReflectProperly(t *testing.T) {
	t.Parallel()
	c1, c2, c3 := 0, 0, 0
	f1, f2, f3 := func() { c1++ }, func() { c2++ }, func() { c3++ }
	t1, t2, t3 := WithNoErr(f1), WithNoErr(f2), WithNoErr(f3)
	combined := t1.Then(t2).Then(t3)

	assert.NoError(t, combined.Run(context.Background()))
	assert.Equal(t, 1, c1)
	assert.Equal(t, 1, c2)
	assert.Equal(t, 1, c3)
}

func TestTask_ThenMustBeImmutable(t *testing.T) {
	t.Parallel()
	t1, t2, t3 := &task{}, &task{}, &task{}
	t1.Then(t2).Then(t3)
	assert.Nil(t, t1.next)
	assert.Nil(t, t2.next)
}

func TestTask_MustRunOrderly(t *testing.T) {
	t.Parallel()

	numbers := make(chan int)
	makeTask := func(ctx context.Context, x int) Task {
		return &task{
			step: func() error {
				numbers <- x
				return nil
			},
			next: nil,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-time.After(time.Second * 5)
		cancel()
	}()

	steps, y, z := makeTask(ctx, 1), makeTask(ctx, 2), makeTask(ctx, 3)
	steps = steps.Then(y).Then(z)

	go func() {
		defer close(numbers)
		assert.NoError(t, steps.Run(ctx))
	}()

	results := make([]int, 0)

	for n := range numbers {
		results = append(results, n)
	}

	for i, v := range results {
		assert.Equal(t, i+1, v)
	}
}

func TestTask_Run_MustPropagateStringPanic(t *testing.T) {
	t.Parallel()
	tt := WithNoErr(func() {
		panic("simpleton")
	})

	err := tt.Run(context.Background())
	assert.Error(t, err)
	assert.ErrorContains(t, err, "simpleton")
}

func TestTask_Run_MustPropagateNonNilNonErrNonStr(t *testing.T) {
	t.Parallel()
	type obj struct {
		msg string
	}
	o := obj{"test message"}
	tt := WithNoErr(func() {
		panic(o)
	})

	err := tt.Run(context.Background())
	assert.Error(t, err)
	assert.ErrorContains(t, err, "test message")
}

func TestTask_Run_MustReportContextError_WhenNotPanic(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	task := With(func() error {
		time.Sleep(time.Second)
		return nil
	})

	eChan := make(chan error, 1)
	go func() { defer close(eChan); eChan <- task.Run(ctx) }()
	cancel()
	err := <-eChan
	assert.ErrorIs(t, err, context.Canceled)

	ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond*50)

	defer func() {
		<-time.After(time.Second)
		cancel()
	}()

	eChan = make(chan error, 1)
	go func() { defer close(eChan); eChan <- task.Run(ctx) }()

	cancel()
	err = <-eChan

	assert.ErrorIs(t, err, context.Canceled)
}

func TestWith_MustPropagateError(t *testing.T) {
	t1, t2 := With(nil), With(func() error { return errors.New("test error") })
	assert.ErrorContains(t, t1.Then(t2).Run(context.Background()), "test error")
}

func TestWith_MustHandleNilArg_AsNoOp(t *testing.T) {
	assert.NotPanics(t, func() {
		tt := With(nil)
		assert.NotNil(t, tt)
		assert.NoError(t, tt.Run(context.Background()))
	})
}

func TestWithNoErr_MustHandleNilArg_AsNoOp(t *testing.T) {
	assert.NotPanics(t, func() {
		tt := WithNoErr(nil)
		assert.NotNil(t, tt)
		assert.NoError(t, tt.Run(context.Background()))
	})
}

func TestTask_Run_MustNotReportContextError_WhenPanic(t *testing.T) {
	t.Parallel()
	tsk := With(func() error {
		panic(context.Canceled)
	})

	assert.ErrorIs(t, tsk.Run(context.Background()), context.Canceled)

	tsk = With(func() error {
		panic(context.DeadlineExceeded)
	})

	assert.ErrorIs(t, tsk.Run(context.Background()), context.DeadlineExceeded)
}

func TestTask_Run_MustReturnError_WhenWorkError(t *testing.T) {
	t.Parallel()
	tsk := With(func() error {
		return errors.New("i am here")
	})

	assert.ErrorContains(t, tsk.Run(context.Background()), "i am here")

	before := With(func() error {
		return nil
	})

	assert.ErrorContains(t, before.Then(tsk).Run(context.Background()), "i am here")
}
