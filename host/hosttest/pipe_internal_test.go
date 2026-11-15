// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest

import (
	"bytes"
	"io"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// result is what a Read or a Write returned.
type result struct {
	n   int
	err error
}

func readAsync(p *pipeEnd, b []byte) <-chan result {
	ch := make(chan result, 1)
	go func() {
		n, err := p.Read(b)
		ch <- result{n, err}
	}()
	return ch
}

func writeAsync(p *pipeEnd, b []byte) <-chan result {
	ch := make(chan result, 1)
	go func() {
		n, err := p.Write(b)
		ch <- result{n, err}
	}()
	return ch
}

func TestPipe_a_Write_returns_before_the_peer_reads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, b := newPipe()
		n, err := a.Write([]byte("hello"))
		require.NoError(t, err)
		require.Equal(t, 5, n)
		require.NoError(t, a.Close())

		got, err := io.ReadAll(b)
		require.NoError(t, err, "the peer reads what was written, then io.EOF")
		require.Equal(t, "hello", string(got))
	})
}

func TestPipe_a_Write_waits_while_the_buffer_is_full(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, b := newPipe()
		want := bytes.Repeat([]byte("x"), pipeBuffer+10)
		wrote := writeAsync(a, want)
		synctest.Wait()
		require.Empty(t, wrote, "the Write returned with the buffer full")

		got := make([]byte, len(want))
		_, err := io.ReadFull(b, got)
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.Equal(t, result{n: len(want)}, <-wrote)
	})
}

func TestPipe_a_Read_waits_for_a_Write(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, b := newPipe()
		buf := make([]byte, 8)
		read := readAsync(b, buf)
		synctest.Wait()
		require.Empty(t, read)

		_, err := a.Write([]byte("hi"))
		require.NoError(t, err)
		require.Equal(t, result{n: 2}, <-read)
		require.Equal(t, "hi", string(buf[:2]))
	})
}

func TestPipe_an_empty_Read_or_Write_returns_at_once(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, b := newPipe()
		n, err := a.Write(nil)
		require.NoError(t, err)
		require.Zero(t, n)

		n, err = b.Read(nil)
		require.NoError(t, err, "an empty Read doesn't wait for a Write")
		require.Zero(t, n)
	})
}

func TestPipe_Close(t *testing.T) {
	t.Run("fails this end's calls with io.ErrClosedPipe", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			a, _ := newPipe()
			require.NoError(t, a.Close())
			require.NoError(t, a.Close(), "closing again does nothing")

			_, err := a.Read(make([]byte, 1))
			require.ErrorIs(t, err, io.ErrClosedPipe)
			_, err = a.Write([]byte("x"))
			require.ErrorIs(t, err, io.ErrClosedPipe)
			require.ErrorIs(t, a.SetDeadline(time.Now()), io.ErrClosedPipe)
			require.ErrorIs(t, a.SetReadDeadline(time.Now()), io.ErrClosedPipe)
			require.ErrorIs(t, a.SetWriteDeadline(time.Now()), io.ErrClosedPipe)
		})
	})

	t.Run("fails the peer's writes with io.ErrClosedPipe", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			a, b := newPipe()
			require.NoError(t, a.Close())
			_, err := b.Write([]byte("x"))
			require.ErrorIs(t, err, io.ErrClosedPipe)
			require.NoError(t, b.SetDeadline(time.Now().Add(time.Second)), "the peer's deadlines can still be set")
		})
	})

	t.Run("wakes the peer's waiting Write", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			a, b := newPipe()
			wrote := writeAsync(b, make([]byte, pipeBuffer+1))
			synctest.Wait()
			require.NoError(t, a.Close())
			require.Equal(t, result{n: pipeBuffer, err: io.ErrClosedPipe}, <-wrote)
		})
	})

	t.Run("wakes the peer's waiting Read with io.EOF", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			a, b := newPipe()
			read := readAsync(b, make([]byte, 1))
			synctest.Wait()
			require.NoError(t, a.Close())
			require.Equal(t, result{err: io.EOF}, <-read)
		})
	})

	t.Run("fails this end's writes before a Read it wakes returns", func(t *testing.T) {
		for range 100 {
			a, b := newPipe()
			wrote := make(chan error, 1)
			go func() {
				_, _ = a.Read(make([]byte, 1))
				_, err := a.Write([]byte("response"))
				wrote <- err
			}()
			require.NoError(t, a.Close())
			require.ErrorIs(t, <-wrote, io.ErrClosedPipe)
			got, err := io.ReadAll(b)
			require.NoError(t, err)
			require.Empty(t, got, "the peer read a write made after the close")
		}
	})

	t.Run("wakes this end's waiting Read and Write", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			a, _ := newPipe()
			read := readAsync(a, make([]byte, 1))
			wrote := writeAsync(a, make([]byte, pipeBuffer+1))
			synctest.Wait()
			require.NoError(t, a.Close())
			require.Equal(t, result{err: io.ErrClosedPipe}, <-read)
			require.Equal(t, result{n: pipeBuffer, err: io.ErrClosedPipe}, <-wrote)
		})
	})
}

func TestPipe_deadlines(t *testing.T) {
	t.Run("a passed read deadline fails a Read even with bytes buffered", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			a, b := newPipe()
			_, err := a.Write([]byte("x"))
			require.NoError(t, err)
			require.NoError(t, b.SetReadDeadline(time.Now()))
			_, err = b.Read(make([]byte, 1))
			require.ErrorIs(t, err, os.ErrDeadlineExceeded)

			require.NoError(t, b.SetReadDeadline(time.Time{}))
			n, err := b.Read(make([]byte, 1))
			require.NoError(t, err, "a deadline of none lets the Read through")
			require.Equal(t, 1, n)
		})
	})

	t.Run("a read deadline wakes a waiting Read", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			_, b := newPipe()
			require.NoError(t, b.SetReadDeadline(time.Now().Add(time.Second)))
			start := time.Now()
			_, err := b.Read(make([]byte, 1))
			require.ErrorIs(t, err, os.ErrDeadlineExceeded)
			require.Equal(t, time.Second, time.Since(start))
		})
	})

	t.Run("a read deadline set while a Read waits applies to it", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			_, b := newPipe()
			require.NoError(t, b.SetReadDeadline(time.Now().Add(time.Hour)))
			read := readAsync(b, make([]byte, 1))
			synctest.Wait()
			require.NoError(t, b.SetReadDeadline(time.Now().Add(time.Second)))
			start := time.Now()
			require.Equal(t, result{err: os.ErrDeadlineExceeded}, <-read)
			require.Equal(t, time.Second, time.Since(start))
		})
	})

	t.Run("a write deadline wakes a Write waiting on a full buffer", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			a, _ := newPipe()
			require.NoError(t, a.SetWriteDeadline(time.Now().Add(time.Second)))
			n, err := a.Write(make([]byte, pipeBuffer+1))
			require.ErrorIs(t, err, os.ErrDeadlineExceeded)
			require.Equal(t, pipeBuffer, n, "the Write reports what it buffered")
		})
	})

	t.Run("a write deadline reset to none while a Write waits leaves it waiting for the peer", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			a, b := newPipe()
			require.NoError(t, a.SetWriteDeadline(time.Now().Add(time.Second)))
			wrote := writeAsync(a, make([]byte, pipeBuffer+1))
			synctest.Wait()
			require.NoError(t, a.SetWriteDeadline(time.Time{}))
			time.Sleep(time.Minute)
			synctest.Wait()
			require.Empty(t, wrote)

			_, err := io.ReadFull(b, make([]byte, pipeBuffer+1))
			require.NoError(t, err)
			require.Equal(t, result{n: pipeBuffer + 1}, <-wrote)
		})
	})

	t.Run("a deadline moved after it passed applies again", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			_, b := newPipe()
			require.NoError(t, b.SetDeadline(time.Now().Add(time.Second)))
			time.Sleep(2 * time.Second)
			require.NoError(t, b.SetDeadline(time.Now().Add(time.Second)))
			start := time.Now()
			_, err := b.Read(make([]byte, 1))
			require.ErrorIs(t, err, os.ErrDeadlineExceeded)
			require.Equal(t, time.Second, time.Since(start))
		})
	})
}

func TestPipe_addresses(t *testing.T) {
	a, _ := newPipe()
	require.Equal(t, "pipe", a.LocalAddr().Network())
	require.Equal(t, "pipe", a.LocalAddr().String())
	require.Equal(t, "pipe", a.RemoteAddr().String())
}
