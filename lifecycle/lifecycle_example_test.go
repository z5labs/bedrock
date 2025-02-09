// Copyright (c) 2025 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package lifecycle

import (
	"context"
	"errors"
	"fmt"
)

func ExampleMultiHook() {
	one := HookFunc(func(ctx context.Context) error {
		fmt.Println("one")
		return nil
	})

	two := HookFunc(func(ctx context.Context) error {
		fmt.Println("two")
		return nil
	})

	mh := MultiHook(one, two)

	err := mh.Run(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Output: one
	// two
}

func ExampleMultiHook_singleError() {
	oneErr := errors.New("one")
	one := HookFunc(func(ctx context.Context) error {
		return oneErr
	})

	two := HookFunc(func(ctx context.Context) error {
		fmt.Println("two")
		return nil
	})

	mh := MultiHook(one, two)

	err := mh.Run(context.Background())
	if err == nil {
		fmt.Println("expected error")
		return
	}

	fmt.Println(errors.Is(err, oneErr))

	// Output: two
	// true
}

func ExampleMultiHook_multipleErrors() {
	oneErr := errors.New("one")
	one := HookFunc(func(ctx context.Context) error {
		return oneErr
	})

	twoErr := errors.New("two")
	two := HookFunc(func(ctx context.Context) error {
		return twoErr
	})

	mh := MultiHook(one, two)

	err := mh.Run(context.Background())
	if err == nil {
		fmt.Println("expected error")
		return
	}

	fmt.Println(errors.Is(err, oneErr), errors.Is(err, twoErr))

	// Output: true true
}

func ExampleContext() {
	ctx := NewContext(context.Background(), &Context{
		PostRun: HookFunc(func(ctx context.Context) error {
			fmt.Println("post run")
			return nil
		}),
	})

	c, ok := FromContext(ctx)
	if !ok {
		fmt.Println()
		return
	}

	err := c.PostRun.Run(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Output: post run
}
