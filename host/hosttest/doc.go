// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package hosttest is a host for tests: an in-memory network and a
// filesystem confined to the test's directory.
//
// A test builds a [Host] with [New], and hands the code under test a
// [github.com/z5labs/bedrock/host.Host] built from the Host's [Host.Listen],
// [Host.Dial] and [Host.FS], in place of the Host of the real machine. The
// code under test listens on the Host's network and the test dials it, and
// every file either opens is inside the test's own temp directory.
//
// The network opens no real socket, starts no goroutines and blocks only
// on channels, so the code can run inside a testing/synctest bubble, with
// no real time, and the filesystem is confined with os.Root to the test's
// temp directory. Build the Host inside the bubble that uses it, with the bubble's t, so that every channel it makes belongs to
// that bubble. Here serve is the code under test, which serves hello over
// HTTP on ":8080" of the host.Host it is given, and the test dials it:
//
//	func TestServe_serves_hello(t *testing.T) {
//		synctest.Test(t, func(t *testing.T) {
//			h := hosttest.New(t)
//			hh := host.Host[FS]{Listen: h.Listen, Dial: h.Dial, FS: h.FS()}
//			ctx, cancel := context.WithCancel(t.Context())
//			defer cancel()
//			done := make(chan error, 1)
//			go func() { done <- serve(ctx, hh) }()
//
//			// Wait until every goroutine in the bubble is blocked, which
//			// includes serve blocked in Accept on :8080.
//			synctest.Wait()
//
//			tr := &http.Transport{DialContext: h.Dial}
//			defer tr.CloseIdleConnections()
//			resp, err := (&http.Client{Transport: tr}).Get("http://localhost:8080/")
//			if err != nil {
//				t.Fatal(err)
//			}
//			// ... read and close the body, and check it ...
//
//			cancel()
//			if err := <-done; err != nil {
//				t.Error(err)
//			}
//		})
//	}
//
// The code reaches the network and the filesystem only through the
// host.Host it is given: it listens with Listen and opens files with FS.
// net.Listen would open a real socket, whose Accept is not durably blocked,
// so synctest.Wait would never return, and os.Open would reach files
// outside the test's directory.
//
// # Names
//
// The rules of the network follow a real one. A name and port can be held
// once, and a listener on every name (an empty host, 0.0.0.0 or ::)
// conflicts with any other listener on its port. Names are compared as
// written and never resolved, so localhost and 127.0.0.1 are different
// names: a client dials the name the code listens on, or the code listens
// on every name, as ":8080" does above.
//
// # Waiting for a listener
//
// [Host.Dial] refuses rather than waits. A Dial with no listener fails at
// once with ECONNREFUSED, as a real dial does, because a test that dials
// before the code listens has dialled too early, and waiting would hide
// that, or turn it into a synctest deadlock report that doesn't name the
// cause. A test waits for the listener itself, before it dials, in one of
// two ways:
//
//   - Inside a synctest bubble, synctest.Wait, which returns once every
//     goroutine in the bubble is blocked, including the code blocked in
//     Accept.
//   - Anywhere, <-h.Listening(address), which waits for one particular
//     listener, like bedrocktest's Process.Notified. [Host.Listening] also
//     works outside a bubble.
//
// # Files
//
// The filesystem is real. [New] takes a directory from t.TempDir(), which
// is removed when the test ends, and [WithFiles] writes the files a test
// sets up before the code starts. The [FS] that [Host.FS] returns opens
// files with an os.Root on that directory, so ".." and symbolic links can't leave it, and
// [Host.Path] gives the absolute path of a file in it to put in an
// argument or a setting. Relative names are relative to the directory,
// which is what the filesystem's [FS.Getwd] returns.
//
// Real files are what let libraries that need an *os.File or a path keep
// working in a test, and they cost two things. A test can't inject an I/O
// error, such as a full disk, from the filesystem alone, so a test that
// needs one embeds *FS in a type of its own and overrides the method that
// fails. And a file's ModTime is real even inside a synctest bubble, whose
// clock is fake, so code that compares ModTime with time.Now needs Chtimes
// in its tests.
//
// # Absolute symbolic links
//
// os.Root never follows a symbolic link to an absolute target, even one
// inside the directory, so such a link would fail at its first use rather
// than where it was made. [FS.Symlink] refuses an absolute target instead,
// creating nothing, with an *os.LinkError whose Err is
// errors.ErrUnsupported, and [FS.CopyFS] fails the same way on an fsys
// that holds one. Code that makes symbolic links makes relative ones, which
// work the same on the real machine and under hosttest.
//
// The design is discussion #614,
// https://github.com/z5labs/bedrock/discussions/614.
package hosttest
