// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

//go:build !linux

package hosttest_test

import (
	"io/fs"
	"time"
)

// accessTime is the zero time off Linux, where the table does not read
// the access time.
func accessTime(fs.FileInfo) time.Time { return time.Time{} }
