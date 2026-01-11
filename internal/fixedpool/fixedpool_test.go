// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package fixedpool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWait_AllTasksSucceed(t *testing.T) {
	ctx := context.Background()

	var counter atomic.Int32
	tasks := []Task{
		func(ctx context.Context) error {
			counter.Add(1)
			return nil
		},
		func(ctx context.Context) error {
			counter.Add(1)
			return nil
		},
		func(ctx context.Context) error {
			counter.Add(1)
			return nil
		},
	}

	err := Wait(ctx, tasks...)
	if err != nil {
		t.Errorf("Wait() error = %v, want nil", err)
	}

	if got := counter.Load(); got != 3 {
		t.Errorf("counter = %d, want 3", got)
	}
}

func TestWait_EmptyTasks(t *testing.T) {
	ctx := context.Background()

	err := Wait(ctx, []Task{}...)
	if err != nil {
		t.Errorf("Wait() error = %v, want nil", err)
	}
}

func TestWait_SingleTask(t *testing.T) {
	ctx := context.Background()

	called := false
	task := func(ctx context.Context) error {
		called = true
		return nil
	}

	err := Wait(ctx, task)
	if err != nil {
		t.Errorf("Wait() error = %v, want nil", err)
	}

	if !called {
		t.Error("task was not called")
	}
}

func TestWait_OneTaskReturnsError(t *testing.T) {
	ctx := context.Background()

	expectedErr := errors.New("task error")

	tasks := []Task{
		func(ctx context.Context) error {
			return nil
		},
		func(ctx context.Context) error {
			return expectedErr
		},
		func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}

	err := Wait(ctx, tasks...)
	if err == nil {
		t.Fatal("Wait() error = nil, want error")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("Wait() error = %v, want error containing %v", err, expectedErr)
	}
}

func TestWait_MultipleTasksReturnErrors(t *testing.T) {
	ctx := context.Background()

	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	tasks := []Task{
		func(ctx context.Context) error {
			return err1
		},
		func(ctx context.Context) error {
			return err2
		},
	}

	err := Wait(ctx, tasks...)
	if err == nil {
		t.Fatal("Wait() error = nil, want error")
	}

	if !errors.Is(err, err1) {
		t.Errorf("Wait() error does not contain err1: %v", err)
	}
	if !errors.Is(err, err2) {
		t.Errorf("Wait() error does not contain err2: %v", err)
	}
}

func TestWait_TaskPanicsWithError(t *testing.T) {
	ctx := context.Background()

	panicErr := errors.New("panic error")

	tasks := []Task{
		func(ctx context.Context) error {
			panic(panicErr)
		},
	}

	err := Wait(ctx, tasks...)
	if err == nil {
		t.Fatal("Wait() error = nil, want error")
	}

	if !errors.Is(err, panicErr) {
		t.Errorf("Wait() error = %v, want error containing %v", err, panicErr)
	}
}

func TestWait_TaskPanicsWithNonError(t *testing.T) {
	ctx := context.Background()

	panicValue := "panic string"

	tasks := []Task{
		func(ctx context.Context) error {
			panic(panicValue)
		},
	}

	err := Wait(ctx, tasks...)
	if err == nil {
		t.Fatal("Wait() error = nil, want error")
	}

	expectedMsg := "recovered from panic: panic string"
	if err.Error() != expectedMsg {
		t.Errorf("Wait() error = %q, want %q", err.Error(), expectedMsg)
	}
}

func TestWait_TaskPanicsAfterReturningError(t *testing.T) {
	ctx := context.Background()

	taskErr := errors.New("task error")
	panicErr := errors.New("panic error")

	tasks := []Task{
		func(ctx context.Context) error {
			defer func() {
				panic(panicErr)
			}()
			return taskErr
		},
	}

	err := Wait(ctx, tasks...)
	if err == nil {
		t.Fatal("Wait() error = nil, want error")
	}

	// When a task panics in a defer after returning, only the panic is captured
	// The return value is lost because the panic occurs during function exit
	if !errors.Is(err, panicErr) {
		t.Errorf("Wait() error does not contain panicErr: %v", err)
	}
}

func TestWait_ContextAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var executed atomic.Bool
	tasks := []Task{
		func(ctx context.Context) error {
			executed.Store(true)
			return nil
		},
	}

	err := Wait(ctx, tasks...)

	// Task should still execute even with pre-canceled context
	// because Wait doesn't check context before starting tasks
	if !executed.Load() {
		t.Error("task was not executed")
	}

	if err != nil {
		t.Errorf("Wait() error = %v, want nil (task completed successfully)", err)
	}
}

func TestWait_ConcurrentExecution(t *testing.T) {
	ctx := context.Background()

	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	const numTasks = 5
	wg.Add(numTasks)

	tasks := make([]Task, numTasks)
	for i := range numTasks {
		tasks[i] = func(ctx context.Context) error {
			wg.Done()
			<-startBarrier
			return nil
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- Wait(ctx, tasks...)
	}()

	// Wait for all tasks to start
	wg.Wait()

	// Release all tasks
	close(startBarrier)

	// Wait for completion with timeout
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Wait() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait() did not complete within timeout")
	}
}

func TestWait_ErrorCancelsOtherTasks(t *testing.T) {
	ctx := context.Background()

	errTask := errors.New("task error")
	var canceled atomic.Bool

	tasks := []Task{
		func(ctx context.Context) error {
			return errTask
		},
		func(ctx context.Context) error {
			<-ctx.Done()
			canceled.Store(true)
			return nil
		},
	}

	err := Wait(ctx, tasks...)
	if err == nil {
		t.Fatal("Wait() error = nil, want error")
	}

	if !errors.Is(err, errTask) {
		t.Errorf("Wait() error = %v, want %v", err, errTask)
	}

	// Give time for cancellation to propagate
	time.Sleep(50 * time.Millisecond)

	if !canceled.Load() {
		t.Error("context was not canceled for other tasks")
	}
}

func TestWait_RaceConditions(t *testing.T) {
	// This test is primarily for running with -race flag
	ctx := context.Background()

	var counter atomic.Int32
	const numTasks = 100

	tasks := make([]Task, numTasks)
	for i := range numTasks {
		tasks[i] = func(ctx context.Context) error {
			counter.Add(1)
			time.Sleep(time.Microsecond)
			return nil
		}
	}

	err := Wait(ctx, tasks...)
	if err != nil {
		t.Errorf("Wait() error = %v, want nil", err)
	}

	if got := counter.Load(); got != numTasks {
		t.Errorf("counter = %d, want %d", got, numTasks)
	}
}

func TestWait_MixedSuccessAndErrors(t *testing.T) {
	ctx := context.Background()

	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	var successCount atomic.Int32

	tasks := []Task{
		func(ctx context.Context) error {
			successCount.Add(1)
			return nil
		},
		func(ctx context.Context) error {
			return err1
		},
		func(ctx context.Context) error {
			successCount.Add(1)
			return nil
		},
		func(ctx context.Context) error {
			return err2
		},
	}

	err := Wait(ctx, tasks...)
	if err == nil {
		t.Fatal("Wait() error = nil, want error")
	}

	if !errors.Is(err, err1) {
		t.Errorf("Wait() error does not contain err1: %v", err)
	}
	if !errors.Is(err, err2) {
		t.Errorf("Wait() error does not contain err2: %v", err)
	}
}

func ExampleWait() {
	ctx := context.Background()

	tasks := []Task{
		func(ctx context.Context) error {
			fmt.Println("Task 1")
			return nil
		},
		func(ctx context.Context) error {
			fmt.Println("Task 2")
			return nil
		},
	}

	err := Wait(ctx, tasks...)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("All tasks completed")
}
