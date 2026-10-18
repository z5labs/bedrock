// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host"
	"github.com/z5labs/bedrock/host/hosttest"
)

// Every test in this file touches files only inside directories from
// t.TempDir(): the Host's own, and a sibling standing in for "outside",
// which every escape attempt aims at and which is checked to be unchanged.

// opener is a filesystem that opens files, such as host.OSFS and
// *hosttest.FS.
type opener interface {
	Open(name string) (*os.File, error)
}

// readAll returns the contents of the named file, opened through fsys.
func readAll(fsys opener, name string) (string, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	return string(b), err
}

// snapshot records every path under dir, with each file's contents, so a
// test can show that nothing under dir was created, changed or removed.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	got := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			got[rel] = "<dir>"
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got[rel] = string(b)
		return nil
	})
	require.NoError(t, err)
	return got
}

// world is a Host with files to work on, and a directory outside it,
// holding a file and a directory, for escapes to aim at.
type world struct {
	host    *hosttest.Host
	fs      *hosttest.FS
	outside string
	before  map[string]string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(outside, "dir"), 0o700))

	h := hosttest.New(t, hosttest.WithFiles(map[string][]byte{
		"dir/file":     []byte("file"),
		"dir/sub/leaf": []byte("leaf"),
	}))
	// An absolute symlink and a relative one, both pointing out of the
	// Host's directory.
	require.NoError(t, os.Symlink(outside, h.Path("out")))
	rel, err := filepath.Rel(h.Path("."), outside)
	require.NoError(t, err)
	require.NoError(t, os.Symlink(rel, h.Path("relout")))

	return &world{host: h, fs: h.FS(), outside: outside, before: snapshot(t, outside)}
}

// escapes returns names for elem outside the Host's directory: through
// "..", as an absolute path, and through each symlink.
func (w *world) escapes(t *testing.T, elem string) map[string]string {
	t.Helper()
	rel, err := filepath.Rel(w.host.Path("."), filepath.Join(w.outside, elem))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(rel, ".."), rel)
	return map[string]string{
		"..":                       rel,
		"absolute outside":         filepath.Join(w.outside, elem),
		"absolute symlink":         filepath.Join("out", elem),
		"relative symlink":         filepath.Join("relout", elem),
		"absolute through symlink": w.host.Path(filepath.ToSlash(filepath.Join("out", elem))),
	}
}

// requireUntouched fails unless the directory outside is as it was.
func (w *world) requireUntouched(t *testing.T) {
	t.Helper()
	require.Equal(t, w.before, snapshot(t, w.outside), "nothing outside the Host's directory changed")
}

// names returns name relative to the Host's directory and as an absolute
// path under it.
func (w *world) names(name string) map[string]string {
	return map[string]string{
		"relative": filepath.FromSlash(name),
		"absolute": w.host.Path(name),
	}
}

// forEach runs f for every name in names, each on a new world.
func forEach(t *testing.T, names func(w *world) map[string]string, f func(t *testing.T, w *world, name string)) {
	t.Helper()
	keys := slices.Sorted(func(yield func(string) bool) {
		for k := range names(newWorld(t)) {
			if !yield(k) {
				return
			}
		}
	})
	for _, k := range keys {
		t.Run(k, func(t *testing.T) {
			w := newWorld(t)
			f(t, w, names(w)[k])
		})
	}
}

// escaping runs f for every escape to elem, and checks that each one
// failed naming the path passed, and left the outside untouched.
func escaping(t *testing.T, elem string, f func(t *testing.T, w *world, name string) error) {
	t.Helper()
	escapingWith(t, elem, func(t *testing.T, err error, name string) {
		require.ErrorContains(t, err, name, "the error names the path passed")
	}, f)
}

// escapingWith is escaping, checking each escape's error with check.
func escapingWith(t *testing.T, elem string, check func(t *testing.T, err error, name string), f func(t *testing.T, w *world, name string) error) {
	t.Helper()
	t.Run("outside", func(t *testing.T) {
		forEach(t, func(w *world) map[string]string { return w.escapes(t, elem) }, func(t *testing.T, w *world, name string) {
			check(t, f(t, w, name), name)
			w.requireUntouched(t)
		})
	})
}

func TestNew_takes_its_directory_from_TempDir(t *testing.T) {
	var dir string
	t.Run("host", func(t *testing.T) {
		h := hosttest.New(t)
		dir = h.Path(".")
		require.True(t, filepath.IsAbs(dir))
		require.Equal(t, filepath.Dir(t.TempDir()), filepath.Dir(dir), "a sibling of the test's other temp directories")
		fi, err := os.Stat(dir)
		require.NoError(t, err)
		require.True(t, fi.IsDir())
	})
	_, err := os.Stat(dir)
	require.ErrorIs(t, err, fs.ErrNotExist, "t.TempDir()'s cleanup removed it")
}

// fatalTB is a testing.TB whose TempDir returns dir, and whose Fatalf
// records its message and stops the goroutine that called it, as
// testing.T's does.
type fatalTB struct {
	testing.TB
	dir   string
	fatal string
}

func (*fatalTB) Helper() {}

func (tb *fatalTB) TempDir() string { return tb.dir }

func (tb *fatalTB) Fatalf(format string, args ...any) {
	tb.fatal = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

func TestNew_fails_the_test_when_it_cannot_open_its_directory(t *testing.T) {
	tb := &fatalTB{TB: t, dir: filepath.Join(t.TempDir(), "missing")}
	var wg sync.WaitGroup
	returned := false
	wg.Go(func() {
		hosttest.New(tb)
		returned = true
	})
	wg.Wait()
	require.False(t, returned, "New stops the test")
	require.Contains(t, tb.fatal, "hosttest: New: ")
	require.Contains(t, tb.fatal, tb.dir)
}

// appFS is the filesystem a framework exposes to its Apps: an interface
// that embeds host.FS and lists the methods it needs.
type appFS interface {
	host.FS
	Open(name string) (*os.File, error)
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	CreateTemp(dir, pattern string) (*os.File, error)
	MkdirTemp(dir, pattern string) (string, error)
	Mkdir(name string, perm os.FileMode) error
	MkdirAll(name string, perm os.FileMode) error
	Remove(name string) error
	RemoveAll(name string) error
	Rename(oldname, newname string) error
	Stat(name string) (os.FileInfo, error)
	ReadDir(name string) ([]os.DirEntry, error)
	DirFS(dir string) fs.FS
}

func TestFS_is_the_filesystem_of_a_host_Host(t *testing.T) {
	h := hosttest.New(t, hosttest.WithFiles(map[string][]byte{"etc/config.yaml": []byte("port: 8080")}))
	hh := host.Host[appFS]{Listen: h.Listen, Dial: h.Dial, FS: h.FS()}

	got, err := readAll(hh.FS, "etc/config.yaml")
	require.NoError(t, err)
	require.Equal(t, "port: 8080", got)
}

func TestFS_embeds_only_host_FS(t *testing.T) {
	typ := reflect.TypeFor[hosttest.FS]()
	var embedded []reflect.Type
	for f := range typ.Fields() {
		require.False(t, f.IsExported(), "FS has no exported fields: %s", f.Name)
		if f.Anonymous {
			embedded = append(embedded, f.Type)
		}
	}
	require.Equal(t, []reflect.Type{reflect.TypeFor[host.FS]()}, embedded,
		"FS gets host.FS's method from host.FS itself, never from host.OSFS, whose methods would escape the directory")
}

func TestWithFiles(t *testing.T) {
	t.Run("writes each file, creating parent directories", func(t *testing.T) {
		h := hosttest.New(t, hosttest.WithFiles(map[string][]byte{
			"top":                []byte("top"),
			"etc/tls/server.p12": []byte("p12"),
			"a/b/c/d":            {},
		}))
		for name, want := range map[string]string{"top": "top", "etc/tls/server.p12": "p12", "a/b/c/d": ""} {
			b, err := os.ReadFile(h.Path(name))
			require.NoError(t, err)
			require.Equal(t, want, string(b), name)
		}
	})

	t.Run("copies the map when it is called", func(t *testing.T) {
		files := map[string][]byte{"a": []byte("before")}
		opt := hosttest.WithFiles(files)
		files["a"][0] = 'X'
		files["b"] = []byte("added")

		h := hosttest.New(t, opt)
		b, err := os.ReadFile(h.Path("a"))
		require.NoError(t, err)
		require.Equal(t, "before", string(b))
		_, err = os.Stat(h.Path("b"))
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("fails the test for a path outside the directory", func(t *testing.T) {
		outside := t.TempDir()
		testCases := map[string]string{
			"absolute":          filepath.ToSlash(filepath.Join(outside, "abs")),
			"..":                "../escaped",
			"leaves on the way": "a/../../escaped",
			"empty":             "",
		}
		for name, path := range testCases {
			t.Run(name, func(t *testing.T) {
				tb := &fakeTB{TB: t}
				h := hosttest.New(tb, hosttest.WithFiles(map[string][]byte{
					path:   []byte("x"),
					"good": []byte("good"),
				}))
				require.Len(t, tb.errors, 1)
				require.Contains(t, tb.errors[0], fmt.Sprintf("%q", path), "the failure names the path")

				_, err := os.Stat(h.Path("good"))
				require.NoError(t, err, "the other files are still written")
				_, err = os.Stat(filepath.Join(filepath.Dir(h.Path(".")), "escaped"))
				require.ErrorIs(t, err, fs.ErrNotExist)
				require.Equal(t, map[string]string{".": "<dir>"}, snapshot(t, outside))
			})
		}
	})

	t.Run("fails the test for a file that cannot be written", func(t *testing.T) {
		tb := &fakeTB{TB: t}
		hosttest.New(tb, hosttest.WithFiles(map[string][]byte{
			"a":   []byte("a file"),
			"a/b": []byte("under a file"),
		}))
		require.Len(t, tb.errors, 1)
		require.Contains(t, tb.errors[0], `"a/b"`)
	})
}

func TestHost_Path(t *testing.T) {
	h := hosttest.New(t, hosttest.WithFiles(map[string][]byte{"etc/tls/server.p12": []byte("p12")}))
	p := h.Path("etc/tls/server.p12")
	require.True(t, filepath.IsAbs(p))
	require.Equal(t, filepath.Join(h.Path("."), "etc", "tls", "server.p12"), p)

	got, err := readAll(h.FS(), p)
	require.NoError(t, err)
	require.Equal(t, "p12", got, "FS().Open of the path opens the file WithFiles wrote")

	t.Run("fails the test for a path outside the directory", func(t *testing.T) {
		for _, name := range []string{"../x", filepath.ToSlash(h.Path("x"))} {
			tb := &fakeTB{TB: t}
			hosttest.New(tb).Path(name)
			require.Len(t, tb.errors, 1)
			require.Contains(t, tb.errors[0], fmt.Sprintf("%q", name))
		}
	})
}

func TestFS_Open(t *testing.T) {
	forEach(t, func(w *world) map[string]string { return w.names("dir/file") }, func(t *testing.T, w *world, name string) {
		got, err := readAll(w.fs, name)
		require.NoError(t, err)
		require.Equal(t, "file", got)
	})

	t.Run("a file's own Name()", func(t *testing.T) {
		w := newWorld(t)
		f, err := w.fs.Open("dir/file")
		require.NoError(t, err)
		require.NoError(t, f.Close())
		require.Equal(t, w.host.Path("dir/file"), f.Name(), "Name() is the absolute OS path")
		got, err := readAll(w.fs, f.Name())
		require.NoError(t, err)
		require.Equal(t, "file", got)
	})

	t.Run("an error names the path passed", func(t *testing.T) {
		w := newWorld(t)
		_, err := w.fs.Open(w.host.Path("missing"))
		var pe *fs.PathError
		require.ErrorAs(t, err, &pe)
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Equal(t, w.host.Path("missing"), pe.Path)
	})

	escaping(t, "secret", func(t *testing.T, w *world, name string) error {
		_, err := readAll(w.fs, name)
		return err
	})
}

func TestFS_OpenFile(t *testing.T) {
	forEach(t, func(w *world) map[string]string { return w.names("dir/new") }, func(t *testing.T, w *world, name string) {
		f, err := w.fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		require.NoError(t, err)
		_, err = io.WriteString(f, "new")
		require.NoError(t, err)
		require.NoError(t, f.Close())

		b, err := os.ReadFile(w.host.Path("dir/new"))
		require.NoError(t, err)
		require.Equal(t, "new", string(b))
	})

	escaping(t, "new", func(t *testing.T, w *world, name string) error {
		f, err := w.fs.OpenFile(name, os.O_RDWR|os.O_CREATE, 0o600)
		if err == nil {
			_ = f.Close()
		}
		return err
	})
}

// requireInside fails unless path is an absolute path under dir.
func requireInside(t *testing.T, dir, path string) {
	t.Helper()
	require.True(t, filepath.IsAbs(path), path)
	rel, err := filepath.Rel(dir, path)
	require.NoError(t, err)
	require.True(t, filepath.IsLocal(rel), "%s is inside %s", path, dir)
}

func TestFS_CreateTemp(t *testing.T) {
	forEach(t, func(w *world) map[string]string { return w.names("dir") }, func(t *testing.T, w *world, dir string) {
		f, err := w.fs.CreateTemp(dir, "pre-*.suf")
		require.NoError(t, err)
		require.NoError(t, f.Close())
		require.Equal(t, w.host.Path("dir"), filepath.Dir(f.Name()))
		base := filepath.Base(f.Name())
		require.True(t, strings.HasPrefix(base, "pre-") && strings.HasSuffix(base, ".suf"), base)

		fi, err := w.fs.Stat(f.Name())
		require.NoError(t, err, "a file's own Name() can be passed back in")
		require.True(t, fi.Mode().IsRegular())
	})

	t.Run(`dir == ""`, func(t *testing.T) {
		w := newWorld(t)
		f, err := w.fs.CreateTemp("", "tmp")
		require.NoError(t, err)
		require.NoError(t, f.Close())
		requireInside(t, w.host.Path("."), f.Name())

		g, err := w.fs.CreateTemp("", "tmp")
		require.NoError(t, err)
		require.NoError(t, g.Close())
		require.NotEqual(t, f.Name(), g.Name(), "every call makes a new file")
		require.Equal(t, filepath.Dir(f.Name()), filepath.Dir(g.Name()))
		w.requireUntouched(t)
	})

	t.Run("a pattern with a path separator", func(t *testing.T) {
		w := newWorld(t)
		_, err := w.fs.CreateTemp("", "../x*")
		require.ErrorContains(t, err, "pattern contains path separator")
	})

	t.Run("a missing dir", func(t *testing.T) {
		for _, dir := range []string{"missing", "missing" + string(filepath.Separator)} {
			w := newWorld(t)
			_, err := w.fs.CreateTemp(dir, "pre-*.suf")
			require.ErrorIs(t, err, fs.ErrNotExist)
			var pe *fs.PathError
			require.ErrorAs(t, err, &pe)
			base, ok := strings.CutPrefix(pe.Path, filepath.Join("missing", "pre-"))
			require.True(t, ok, "the error names the dir passed, as os.CreateTemp's does: %s", pe.Path)
			require.True(t, strings.HasSuffix(base, ".suf"), pe.Path)
		}
	})

	t.Run(`dir == "" when tmp is a file`, func(t *testing.T) {
		h := hosttest.New(t, hosttest.WithFiles(map[string][]byte{"tmp": []byte("in the way")}))
		_, err := h.FS().CreateTemp("", "x")
		require.Error(t, err)
	})

	escaping(t, "dir", func(t *testing.T, w *world, dir string) error {
		f, err := w.fs.CreateTemp(dir, "x")
		if err == nil {
			_ = f.Close()
		}
		return err
	})
}

func TestFS_MkdirTemp(t *testing.T) {
	forEach(t, func(w *world) map[string]string { return w.names("dir") }, func(t *testing.T, w *world, dir string) {
		name, err := w.fs.MkdirTemp(dir, "pre-*.suf")
		require.NoError(t, err)
		require.Equal(t, w.host.Path("dir"), filepath.Dir(name))
		base := filepath.Base(name)
		require.True(t, strings.HasPrefix(base, "pre-") && strings.HasSuffix(base, ".suf"), base)

		fi, err := w.fs.Stat(name)
		require.NoError(t, err)
		require.True(t, fi.IsDir())
	})

	t.Run(`dir == ""`, func(t *testing.T) {
		w := newWorld(t)
		a, err := w.fs.MkdirTemp("", "tmp")
		require.NoError(t, err)
		requireInside(t, w.host.Path("."), a)
		b, err := w.fs.MkdirTemp("", "tmp")
		require.NoError(t, err)
		require.NotEqual(t, a, b)
		w.requireUntouched(t)
	})

	t.Run("a pattern with a path separator", func(t *testing.T) {
		w := newWorld(t)
		_, err := w.fs.MkdirTemp("", "a/b")
		require.ErrorContains(t, err, "pattern contains path separator")
	})

	t.Run("a missing dir", func(t *testing.T) {
		w := newWorld(t)
		_, err := w.fs.MkdirTemp("missing", "x")
		require.ErrorIs(t, err, fs.ErrNotExist)
		var pe *fs.PathError
		require.ErrorAs(t, err, &pe)
		require.Equal(t, "missing", pe.Path, "the error names the dir passed, as os.MkdirTemp's does")
	})

	t.Run(`dir == "" when tmp is a file`, func(t *testing.T) {
		h := hosttest.New(t, hosttest.WithFiles(map[string][]byte{"tmp": []byte("in the way")}))
		_, err := h.FS().MkdirTemp("", "x")
		require.Error(t, err)
	})

	escaping(t, "dir", func(t *testing.T, w *world, dir string) error {
		_, err := w.fs.MkdirTemp(dir, "x")
		return err
	})
}

func TestFS_Mkdir(t *testing.T) {
	forEach(t, func(w *world) map[string]string { return w.names("dir/new") }, func(t *testing.T, w *world, name string) {
		require.NoError(t, w.fs.Mkdir(name, 0o700))
		fi, err := os.Stat(w.host.Path("dir/new"))
		require.NoError(t, err)
		require.True(t, fi.IsDir())
	})

	escaping(t, "new", func(t *testing.T, w *world, name string) error {
		return w.fs.Mkdir(name, 0o700)
	})
}

func TestFS_MkdirAll(t *testing.T) {
	forEach(t, func(w *world) map[string]string { return w.names("a/b/c") }, func(t *testing.T, w *world, name string) {
		require.NoError(t, w.fs.MkdirAll(name, 0o700))
		fi, err := os.Stat(w.host.Path("a/b/c"))
		require.NoError(t, err)
		require.True(t, fi.IsDir())
	})

	escaping(t, "a/b", func(t *testing.T, w *world, name string) error {
		return w.fs.MkdirAll(name, 0o700)
	})
}

func TestFS_Remove(t *testing.T) {
	forEach(t, func(w *world) map[string]string { return w.names("dir/file") }, func(t *testing.T, w *world, name string) {
		require.NoError(t, w.fs.Remove(name))
		_, err := os.Stat(w.host.Path("dir/file"))
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	escaping(t, "secret", func(t *testing.T, w *world, name string) error {
		return w.fs.Remove(name)
	})
}

func TestFS_RemoveAll(t *testing.T) {
	forEach(t, func(w *world) map[string]string { return w.names("dir") }, func(t *testing.T, w *world, name string) {
		require.NoError(t, w.fs.RemoveAll(name))
		_, err := os.Stat(w.host.Path("dir"))
		require.ErrorIs(t, err, fs.ErrNotExist)
		w.requireUntouched(t)
	})

	t.Run("a symlink out is removed, not followed", func(t *testing.T) {
		w := newWorld(t)
		require.NoError(t, w.fs.RemoveAll("out"))
		_, err := os.Lstat(w.host.Path("out"))
		require.ErrorIs(t, err, fs.ErrNotExist)
		w.requireUntouched(t)
	})

	t.Run("outside", func(t *testing.T) {
		w := newWorld(t)
		rel, err := filepath.Rel(w.host.Path("."), w.outside)
		require.NoError(t, err)
		for _, name := range []string{rel, w.outside, filepath.Join(w.outside, "secret"), filepath.Join("out", "secret"), filepath.Join("relout", "dir")} {
			require.Error(t, w.fs.RemoveAll(name), name)
			w.requireUntouched(t)
		}
	})
}

func TestFS_Rename(t *testing.T) {
	forEach(t, func(w *world) map[string]string {
		return map[string]string{
			"relative": "dir/file|moved",
			"absolute": w.host.Path("dir/file") + "|" + w.host.Path("moved"),
		}
	}, func(t *testing.T, w *world, names string) {
		oldname, newname, _ := strings.Cut(names, "|")
		require.NoError(t, w.fs.Rename(oldname, newname))
		b, err := os.ReadFile(w.host.Path("moved"))
		require.NoError(t, err)
		require.Equal(t, "file", string(b))
		_, err = os.Stat(w.host.Path("dir/file"))
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("an error names the paths passed", func(t *testing.T) {
		w := newWorld(t)
		oldname, newname := w.host.Path("missing"), w.host.Path("moved")
		err := w.fs.Rename(oldname, newname)
		var le *os.LinkError
		require.ErrorAs(t, err, &le)
		require.Equal(t, oldname, le.Old)
		require.Equal(t, newname, le.New)
	})

	escaping(t, "secret", func(t *testing.T, w *world, name string) error {
		return w.fs.Rename(name, "stolen")
	})
	t.Run("into", func(t *testing.T) {
		escaping(t, "planted", func(t *testing.T, w *world, name string) error {
			return w.fs.Rename("dir/file", name)
		})
	})
}

func TestFS_Stat(t *testing.T) {
	forEach(t, func(w *world) map[string]string { return w.names("dir/file") }, func(t *testing.T, w *world, name string) {
		fi, err := w.fs.Stat(name)
		require.NoError(t, err)
		require.Equal(t, "file", fi.Name())
		require.EqualValues(t, len("file"), fi.Size())
	})

	escaping(t, "secret", func(t *testing.T, w *world, name string) error {
		_, err := w.fs.Stat(name)
		return err
	})
}

func TestFS_ReadDir(t *testing.T) {
	forEach(t, func(w *world) map[string]string { return w.names("dir") }, func(t *testing.T, w *world, name string) {
		entries, err := w.fs.ReadDir(name)
		require.NoError(t, err)
		var got []string
		for _, e := range entries {
			got = append(got, e.Name())
		}
		require.Equal(t, []string{"file", "sub"}, got, "sorted by filename")
	})

	t.Run("a file", func(t *testing.T) {
		forEach(t, func(w *world) map[string]string { return w.names("dir/file") }, func(t *testing.T, w *world, name string) {
			_, err := w.fs.ReadDir(name)
			var pe *fs.PathError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, name, pe.Path, "the error names the path passed")
		})
	})

	escaping(t, "dir", func(t *testing.T, w *world, name string) error {
		_, err := w.fs.ReadDir(name)
		return err
	})
}

func TestFS_DirFS(t *testing.T) {
	forEach(t, func(w *world) map[string]string { return w.names("dir") }, func(t *testing.T, w *world, dir string) {
		var got []string
		err := fs.WalkDir(w.fs.DirFS(dir), ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			got = append(got, path)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, []string{".", "file", "sub", "sub/leaf"}, got)

		b, err := fs.ReadFile(w.fs.DirFS(dir), "sub/leaf")
		require.NoError(t, err)
		require.Equal(t, "leaf", string(b))
	})

	t.Run("the whole directory, through a symlink out", func(t *testing.T) {
		w := newWorld(t)
		var walked []string
		err := fs.WalkDir(w.fs.DirFS("."), ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			walked = append(walked, path)
			return nil
		})
		require.NoError(t, err, "WalkDir does not follow the symlinks")
		require.Contains(t, walked, "out")
		_, err = fs.ReadFile(w.fs.DirFS("."), "out/secret")
		require.Error(t, err)
	})

	t.Run("a relative dir that is not clean", func(t *testing.T) {
		for _, dir := range []string{"./dir", "dir/", "dir/../dir", "dir/sub/.."} {
			w := newWorld(t)
			b, err := fs.ReadFile(w.fs.DirFS(dir), "sub/leaf")
			require.NoError(t, err, dir)
			require.Equal(t, "leaf", string(b), dir)
		}
	})

	t.Run(`dir == ""`, func(t *testing.T) {
		w := newWorld(t)
		_, err := w.fs.DirFS("").Open("dir")
		require.Error(t, err, "as os.DirFS(\"\") fails, rather than serving the whole directory")
	})

	// Through a symbolic link, the dir is inside the Host's directory, and
	// it is the fs.FS that refuses to follow the link out. Its errors name
	// paths inside the fs.FS, as os.DirFS's do, so the dir passed is not in
	// them.
	escapingWith(t, "dir", func(t *testing.T, err error, _ string) { require.Error(t, err) }, func(t *testing.T, w *world, dir string) error {
		return fs.WalkDir(w.fs.DirFS(dir), ".", func(path string, d fs.DirEntry, err error) error {
			return err
		})
	})
}

func TestHost_parallel_Hosts_never_see_each_others_files(t *testing.T) {
	const hosts = 4
	for i := range hosts {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			mine := fmt.Sprintf("host-%d", i)
			h := hosttest.New(t, hosttest.WithFiles(map[string][]byte{"shared": []byte(mine)}))
			fsys := h.FS()
			for j := range 20 {
				f, err := fsys.OpenFile(fmt.Sprintf("%s-%d", mine, j), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
				require.NoError(t, err)
				require.NoError(t, f.Close())

				got, err := readAll(fsys, "shared")
				require.NoError(t, err)
				require.Equal(t, mine, got)

				entries, err := fsys.ReadDir(".")
				require.NoError(t, err)
				for _, e := range entries {
					require.True(t, e.Name() == "shared" || strings.HasPrefix(e.Name(), mine+"-"), "host %d sees %s", i, e.Name())
				}
			}
		})
	}
}
