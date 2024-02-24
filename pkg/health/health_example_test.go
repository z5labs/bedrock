// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package health

import (
	"context"
	"fmt"
)

func ExampleBinary() {
	var b Binary
	fmt.Println(b.Healthy(context.Background()))

	b.Toggle()
	fmt.Println(b.Healthy(context.Background()))
	// Output: true
	// false
}

func ExampleAnd() {
	var a Binary
	var b Binary
	b.Toggle()

	ab := And(&a, &b)
	fmt.Println(ab.Healthy(context.Background()))
	// Output: false
}

func ExampleOr() {
	var a Binary
	var b Binary
	b.Toggle()

	ob := Or(&a, &b)
	fmt.Println(ob.Healthy(context.Background()))
	// Output: true
}

func ExampleNot() {
	var b Binary

	nb := Not(&b)

	fmt.Println(b.Healthy(context.Background()))
	fmt.Println(nb.Healthy(context.Background()))
	// Output: true
	// false
}
