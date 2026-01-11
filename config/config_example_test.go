// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
)

func Example() {
	myIntFromString, _ := Read(
		context.Background(),
		Default(10, Int64FromString(Env("MY_INT"))),
	)

	myIntFromBytes, _ := Read(
		context.Background(),
		Default(10, Int64FromBytes(binary.LittleEndian, ReaderOf(bytes.NewReader([]byte{1, 1, 1, 1, 1, 1, 1, 1})))),
	)

	fmt.Println(myIntFromString)
	fmt.Println(myIntFromBytes)
	// Output:
	// 10
	// 72340172838076673
}
