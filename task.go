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

		// run step from THIS task.
		if err := t.step(); err != nil {
			errChan <- err
			return
		}

		// check and run next step if exists
		if t.next != nil {
			if err := t.next.Run(ctx); err != nil {
				errChan <- err // propagation for error
				return
			}
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
