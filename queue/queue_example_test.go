// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package queue

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

func ExamplePipe() {
	var i int
	c := ConsumerFunc[int](func(ctx context.Context) (*Item[int], error) {
		i += 1
		if i == 5 {
			return nil, ErrEndOfItems
		}
		item := &Item[int]{
			Value: i,
		}
		return item, nil
	})

	var mu sync.Mutex
	var ints []int
	p := ProcessorFunc[int](func(ctx context.Context, item int) error {
		mu.Lock()
		defer mu.Unlock()
		ints = append(ints, item)
		return nil
	})

	rt := NewRuntime(Pipe[int](c, p))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := rt.Run(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
	is := sort.IntSlice(ints)
	sort.Sort(is)
	fmt.Println(is)
	//Output: [1 2 3 4]
}

func ExamplePipe_maxConcurrentProcessors() {
	var i int
	c := ConsumerFunc[int](func(ctx context.Context) (*Item[int], error) {
		i += 1
		if i == 5 {
			return nil, ErrEndOfItems
		}
		item := &Item[int]{
			Value: i,
		}
		return item, nil
	})

	var mu sync.Mutex
	var ints []int
	p := ProcessorFunc[int](func(ctx context.Context, item int) error {
		mu.Lock()
		defer mu.Unlock()
		ints = append(ints, item)
		return nil
	})

	rt := NewRuntime(
		Pipe[int](c, p),
		MaxConcurrentProcessors(1),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := rt.Run(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
	is := sort.IntSlice(ints)
	sort.Sort(is)
	fmt.Println(is)
	//Output: [1 2 3 4]
}

func ExampleSequential() {
	var i int
	c := ConsumerFunc[int](func(ctx context.Context) (*Item[int], error) {
		i += 1
		if i == 5 {
			return nil, ErrEndOfItems
		}
		item := &Item[int]{
			Value: i,
		}
		return item, nil
	})

	var ints []int
	p := ProcessorFunc[int](func(ctx context.Context, item int) error {
		ints = append(ints, item)
		return nil
	})

	rt := NewRuntime(Sequential[int](c, p))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := rt.Run(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
	is := sort.IntSlice(ints)
	sort.Sort(is)
	fmt.Println(is)
	//Output: [1 2 3 4]
}
