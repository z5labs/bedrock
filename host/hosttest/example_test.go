// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/z5labs/bedrock/host/hosttest"
)

// A test waits for a listener before it dials. Inside a synctest bubble,
// synctest.Wait does that for every listener at once; Listening waits for
// one, and works outside a bubble too. A listener on every name, such as
// ":8080", is reached by a Dial to any name on its port.
func ExampleHost_Listening() {
	t := new(exampleTB)
	defer t.runCleanups()

	h := hosttest.New(t)
	_, err := h.Dial(context.Background(), "tcp", "localhost:8080")
	fmt.Println("before Listen, refused:", errors.Is(err, syscall.ECONNREFUSED))

	// The goroutine stands in for the code under test, which listens when
	// it gets to it, and closes its listener once it has served. Listening
	// is called before the goroutine starts, so the listener can't come and
	// go before the test looks for it. Once a listener exists, Listening
	// returns a channel that is already closed.
	listening := h.Listening("localhost:8080")
	served := make(chan struct{})
	go func() {
		defer close(served)
		ln, err := h.Listen("tcp", ":8080")
		if err != nil {
			fmt.Println(err)
			return
		}
		defer func() { _ = ln.Close() }()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.WriteString(conn, "hello from every name")
	}()

	select {
	case <-listening:
	case <-served: // Listen failed, and the goroutine said why
		return
	}
	conn, err := h.Dial(context.Background(), "tcp", "localhost:8080")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = conn.Close() }()
	b, err := io.ReadAll(conn)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(b))
	<-served
	// Output:
	// before Listen, refused: true
	// hello from every name
}

// WithFiles writes the files a test sets up into the Host's directory
// before the code under test starts, and Path gives the absolute OS path
// of one, to put in an argument or a setting. The Host's FS opens it
// there, and refuses a file that exists outside the directory.
func ExampleWithFiles() {
	t := new(exampleTB)
	defer t.runCleanups()

	h := hosttest.New(t, hosttest.WithFiles(map[string][]byte{
		"etc/app/greeting.txt": []byte("hello from a real file"),
	}))
	path := h.Path("etc/app/greeting.txt")
	fmt.Println("absolute:", filepath.IsAbs(path))

	f, err := h.FS().Open(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(b))

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		fmt.Println(err)
		return
	}
	_, err = os.Stat(outside)
	fmt.Println("outside exists:", err == nil)
	_, err = h.FS().Open(outside)
	fmt.Println("outside the directory refused:", err != nil)
	// Output:
	// absolute: true
	// hello from a real file
	// outside exists: true
	// outside the directory refused: true
}
