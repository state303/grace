package grace

import (
	"context"
	"errors"
	"fmt"
)

// Task is an abstraction that represents a single task.
// This may, or may not have chained tasks
type Task interface {
	// Run with context. This context will be propagated to every chained task.
	Run(ctx context.Context) error

	// Then chains this Task with that Task; every each as a new, copied Task instance.
	// The action is immutable, hence does not affect caller.
	Then(that Task) Task

	// Step returns a grace.Step instance that is assigned to this Task instance.
	Step() Step

	// Next returns a grace.Task instance that is assigned as next task from this Task.
	Next() Task
}

// With returns new Task instance
func With(step Step) Task {
	if step == nil {
		step = func() error { return nil }
	}
	return &task{step, nil}
}

// WithNoErr returns new Task that always returns nil
func WithNoErr(step func()) Task {
	if step == nil {
		step = func() {}
	}
	return With(func() error {
		step()
		return nil
	})
}

type Step func() error

// task as an implementation of Task
type task struct {
	step Step
	next Task
}

// Run implement Task.Run
func (t *task) Run(ctx context.Context) error {
	errChan, done := make(chan error, 1), make(chan struct{}, 1)
	go func() {
		// handle panic if any, then deferring close for channels
		defer func() {
			if p := recover(); p != nil { // check panic content
				// check if panic content is either an error or a string
				if err, ok := p.(error); ok { // error
					errChan <- err
				} else if str, isStr := p.(string); isStr { // string
					errChan <- errors.New(str)
				} else { // not nil, not error, not string
					errChan <- fmt.Errorf("%+v", p)
				}
			}
			defer close(errChan)
			defer close(done)
		}()

		// assign initial step
		var tt Task
		tt = t

	dig:
		if ctx.Err() != nil { // context canceled or deadline exceeded, etc
			return
		}
		step := tt.Step()

		if err := step(); err != nil {
			errChan <- err
			return
		}

		// check and dig next step if exists
		if tt.Next() != nil {
			tt = tt.Next()
			goto dig
		}

		// observed no error, thus signal success
		done <- struct{}{}
	}()

	select {
	case <-ctx.Done(): // context done will always be faster if done ever happens
		return ctx.Err()
	case err := <-errChan: // propagate error
		return err
	case <-done: // returns nil as we observed no error thus far
		return nil
	}
}

// Then implements Task.Then
func (t *task) Then(next Task) Task {
	// always copy a task into a new instance of task.
	cp := &task{
		step: t.step,
		next: t.next,
	}
	// assign next task accordingly.
	if cp.next == nil {
		cp.next = next
	} else {
		cp.next = cp.next.Then(next) // keep immutability
	}
	return cp
}

func (t *task) Step() Step {
	return t.step
}

func (t *task) Next() Task {
	return t.next
}
