// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import "fmt"

func ExampleRead() {
	src := Map{
		"hello": "world",
	}

	m, err := Read(src)
	if err != nil {
		fmt.Println(err)
		return
	}

	var cfg struct {
		Hello string `config:"hello"`
	}
	err = m.Unmarshal(&cfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(cfg.Hello)
	// Output: world
}
