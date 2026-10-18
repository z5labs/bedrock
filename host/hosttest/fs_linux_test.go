// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"io/fs"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host/hosttest"
)

// accessTime returns the access time of fi, which only the OS's own
// FileInfo.Sys holds. Off Linux it is the zero time (fs_other_test.go).
func accessTime(fi fs.FileInfo) time.Time {
	st := fi.Sys().(*syscall.Stat_t)
	return time.Unix(st.Atim.Unix()).UTC()
}

func TestFS_Chtimes_sets_the_access_time(t *testing.T) {
	h := hosttest.New(t, hosttest.WithFiles(map[string][]byte{"file": []byte("file")}))
	atime := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
	mtime := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	require.NoError(t, h.FS().Chtimes("file", atime, mtime))
	fi, err := h.FS().Stat("file")
	require.NoError(t, err)
	require.Equal(t, atime, accessTime(fi))
	require.Equal(t, mtime, fi.ModTime().UTC())
}

func TestFS_Truncate_a_named_pipe_fails_without_waiting_for_a_reader(t *testing.T) {
	h := hosttest.New(t)
	require.NoError(t, syscall.Mkfifo(h.Path("pipe"), 0o600))
	done := make(chan error, 1)
	go func() { done <- h.FS().Truncate("pipe", 0) }()
	select {
	case err := <-done:
		var pe *fs.PathError
		require.ErrorAs(t, err, &pe)
		require.Equal(t, "truncate", pe.Op)
		require.Equal(t, "pipe", pe.Path)
	case <-time.After(10 * time.Second):
		t.Fatal("Truncate of a named pipe is waiting for a reader")
	}
}
