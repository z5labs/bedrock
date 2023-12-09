// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func ExampleRead_yaml() {
	r := strings.NewReader(`hello: world`)

	m, err := Read(r, Language(YAML))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(m.GetString("hello"))
	// Output: world
}

func ExampleRead_json() {
	r := strings.NewReader(`{"hello": "world"}`)

	m, err := Read(r, Language(JSON))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(m.GetString("hello"))
	// Output: world
}

func ExampleRead_toml() {
	r := strings.NewReader(`hello = "world"`)

	m, err := Read(r, Language(TOML))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(m.GetString("hello"))
	// Output: world
}

func ExampleRead_env() {
	os.Setenv("HELLO", "world")
	defer os.Unsetenv("HELLO")

	r := strings.NewReader(`hello: {{env "HELLO"}}`)

	m, err := Read(r, Language(YAML))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(m.GetString("hello"))
	// Output: world
}

func ExampleRead_envWithDefault() {
	r := strings.NewReader(`hello: {{env "HELLO" | default "world"}}`)

	m, err := Read(r, Language(YAML))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(m.GetString("hello"))
	// Output: world
}

func ExampleManager_Unmarshal() {
	r := strings.NewReader(`hello: world
duration: 10s
n: 2
f: 3.14`)

	m, err := Read(r, Language(YAML))
	if err != nil {
		fmt.Println(err)
		return
	}

	var cfg struct {
		Hello    string        `config:"hello"`
		Duration time.Duration `config:"duration"`
		N        int           `config:"n"`
		F        float64       `config:"f"`
	}
	err = m.Unmarshal(&cfg)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(cfg.Hello)
	fmt.Println(cfg.Duration)
	fmt.Println(cfg.N)
	fmt.Println(cfg.F)
	// Output: world
	// 10s
	// 2
	// 3.14
}
