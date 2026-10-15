// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"net"

	"github.com/z5labs/bedrock"
)

func main() {
	app := App{
		Listen:  net.Listen,
		Handler: NewHandler,
	}
	bedrock.Main(app)
}
