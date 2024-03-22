// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package configtmpl

import (
	"fmt"
	"os"
)

func ExampleEnv() {
	os.Setenv("HELLO", "WORLD")
	defer os.Unsetenv("HELLO")

	fmt.Println(Env("HELLO"))
	// Output: WORLD
}

func ExampleDefault() {
	def := "good bye"
	v := "hello world"

	fmt.Println(Default(def, v))
	// Output: hello world
}

func ExampleDefault_nil() {
	fmt.Println(Default("WORLD", nil))
	// Output: WORLD
}

func ExampleDefault_zero() {
	var v int
	fmt.Println(Default(10, v))
	// Output: 10
}
