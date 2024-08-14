// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"fmt"
	"os"
	"strings"
)

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

func ExampleRead_map() {
	src := Map{
		"hello": "world",
		"xs":    []int{1, 2, 3},
		"a": map[string]any{
			"b": 1.2,
		},
	}

	m, err := Read(src)
	if err != nil {
		fmt.Println(err)
		return
	}

	var cfg struct {
		Hello string `config:"hello"`
		Xs    []int  `config:"xs"`
		A     struct {
			B float64 `config:"b"`
		} `config:"a"`
	}
	err = m.Unmarshal(&cfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(cfg.Hello)
	fmt.Println(cfg.Xs)
	fmt.Println(cfg.A.B)
	// Output: world
	// [1 2 3]
	// 1.2
}

func ExampleRead_yaml() {
	r := strings.NewReader(`
hello: world
xs:
  - 1
  - 2
  - 3
a:
  b: 1.2
`)
	src := FromYaml(r)

	m, err := Read(src)
	if err != nil {
		fmt.Println(err)
		return
	}

	var cfg struct {
		Hello string `config:"hello"`
		Xs    []int  `config:"xs"`
		A     struct {
			B float64 `config:"b"`
		} `config:"a"`
	}
	err = m.Unmarshal(&cfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(cfg.Hello)
	fmt.Println(cfg.Xs)
	fmt.Println(cfg.A.B)
	// Output: world
	// [1 2 3]
	// 1.2
}

func ExampleRead_json() {
	r := strings.NewReader(`{
		"hello": "world",
		"xs":[
		  1,
		  2,
		  3
		],
		"a":{
		  "b": 1.2
		}
	}`)
	src := FromJson(r)

	m, err := Read(src)
	if err != nil {
		fmt.Println(err)
		return
	}

	var cfg struct {
		Hello string `config:"hello"`
		Xs    []int  `config:"xs"`
		A     struct {
			B float64 `config:"b"`
		} `config:"a"`
	}
	err = m.Unmarshal(&cfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(cfg.Hello)
	fmt.Println(cfg.Xs)
	fmt.Println(cfg.A.B)
	// Output: world
	// [1 2 3]
	// 1.2
}

func ExampleRead_env() {
	os.Setenv("HELLO", "world")
	defer os.Unsetenv("HELLO")

	src := FromEnv()

	m, err := Read(src)
	if err != nil {
		fmt.Println(err)
		return
	}

	var cfg struct {
		Hello string `config:"HELLO"`
	}
	err = m.Unmarshal(&cfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(cfg.Hello)
	// Output: world
}

func ExampleRead_textTemplateRenderer() {
	r := strings.NewReader(`hello: {{ myName }}`)
	ttr := RenderTextTemplate(
		r,
		TemplateFunc("myName", func() string {
			return "bob"
		}),
	)

	src := FromYaml(ttr)
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
	// Output: bob
}

func ExampleRead_textTemplateRenderer_CustomDelims() {
	r := strings.NewReader(`hello: (( myName ))`)
	ttr := RenderTextTemplate(
		r,
		TemplateDelims("((", "))"),
		TemplateFunc("myName", func() string {
			return "bob"
		}),
	)

	src := FromYaml(ttr)
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
	// Output: bob
}
