// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host"
	"github.com/z5labs/bedrock/host/hosttest"
)

// conformanceFS lists the methods the FS conformance table checks
// *hosttest.FS has, with the same results as host.OSFS.
type conformanceFS interface {
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

// conformanceFiles is the directory every scenario starts from, on both
// filesystems.
var conformanceFiles = map[string][]byte{
	"dir/file":     []byte("file"),
	"dir/sub/leaf": []byte("leaf"),
	"empty/.keep":  {},
}

// fsScenario is one row of the FS conformance table. It runs on fsys, whose
// directory is dir, an absolute path holding conformanceFiles, and returns
// what it observed. Every name it passes is absolute under dir, because
// host.OSFS resolves a relative name against the process's working
// directory.
type fsScenario func(t *testing.T, fsys conformanceFS, dir string) any

// TestConformance_FS runs every scenario on host.OSFS, in a directory from
// t.TempDir(), and on *hosttest.FS, in its Host's directory, and requires
// the two to observe the same.
//
// What a scenario observes is the same when the return values are: file
// contents, a FileInfo's name, size, mode and IsDir, and sorted directory
// entries. An error is the same when it has the same type, names the same
// path relative to the directory, and is the same errors.Is class. Its Op
// may differ, since os.Root's ops are openat and the like. So may the
// random names CreateTemp and MkdirTemp pick: for those, a scenario
// checks that each result is a new entry in the directory passed,
// matching the pattern's prefix and suffix.
func TestConformance_FS(t *testing.T) {
	testCases := map[string]fsScenario{
		"Open a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			return read(t, fsys, filepath.Join(dir, "dir", "file"))
		},
		"Open a missing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.Open(filepath.Join(dir, "missing"))
			return describe(t, dir, err)
		},
		"Open a missing directory's file": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.Open(filepath.Join(dir, "missing", "file"))
			return describe(t, dir, err)
		},

		"OpenFile creates a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "new")
			f, err := fsys.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			require.NoError(t, err)
			_, err = io.WriteString(f, "new")
			require.NoError(t, err)
			require.NoError(t, f.Close())
			return []any{read(t, fsys, name), stat(t, fsys, name)}
		},
		"OpenFile appends to a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "file")
			f, err := fsys.OpenFile(name, os.O_WRONLY|os.O_APPEND, 0)
			require.NoError(t, err)
			_, err = io.WriteString(f, "+more")
			require.NoError(t, err)
			require.NoError(t, f.Close())
			return read(t, fsys, name)
		},
		"OpenFile with O_EXCL on an existing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.OpenFile(filepath.Join(dir, "dir", "file"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			return describe(t, dir, err)
		},
		"OpenFile without O_CREATE on a missing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.OpenFile(filepath.Join(dir, "missing"), os.O_RDWR, 0)
			return describe(t, dir, err)
		},

		"CreateTemp in a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			in := filepath.Join(dir, "dir")
			f, err := fsys.CreateTemp(in, "pre-*.suf")
			require.NoError(t, err)
			_, err = io.WriteString(f, "temp")
			require.NoError(t, err)
			require.NoError(t, f.Close())
			return []any{newEntry(t, fsys, in, f.Name(), "pre-", ".suf"), read(t, fsys, f.Name())}
		},
		"CreateTemp twice makes two files": func(t *testing.T, fsys conformanceFS, dir string) any {
			in := filepath.Join(dir, "dir")
			var created []string
			for range 2 {
				f, err := fsys.CreateTemp(in, "x")
				require.NoError(t, err)
				require.NoError(t, f.Close())
				created = append(created, f.Name())
			}
			require.NotEqual(t, created[0], created[1])
			return entries(t, fsys, in, func(name string) string {
				if strings.HasPrefix(name, "x") {
					return "x*"
				}
				return name
			})
		},
		"CreateTemp in a missing directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			in := filepath.Join(dir, "missing")
			_, err := fsys.CreateTemp(in, "pre-*.suf")
			return describe(t, dir, err, tempPath(in, "pre-", ".suf"))
		},
		"CreateTemp with a path separator in the pattern": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.CreateTemp(filepath.Join(dir, "dir"), "a/b*")
			return describe(t, dir, err)
		},

		"MkdirTemp in a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			in := filepath.Join(dir, "dir")
			name, err := fsys.MkdirTemp(in, "pre-*.suf")
			require.NoError(t, err)
			return []any{newEntry(t, fsys, in, name, "pre-", ".suf"), stat(t, fsys, name).IsDir}
		},
		"MkdirTemp in a missing directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			in := filepath.Join(dir, "missing")
			_, err := fsys.MkdirTemp(in, "pre-*.suf")
			return describe(t, dir, err, tempPath(in, "pre-", ".suf"))
		},
		"MkdirTemp with a path separator in the pattern": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.MkdirTemp(filepath.Join(dir, "dir"), "a/b*")
			return describe(t, dir, err)
		},

		"Mkdir": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "new")
			require.NoError(t, fsys.Mkdir(name, 0o750))
			return stat(t, fsys, name)
		},
		"Mkdir an existing directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Mkdir(filepath.Join(dir, "dir"), 0o750))
		},
		"Mkdir in a missing directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Mkdir(filepath.Join(dir, "missing", "new"), 0o750))
		},

		"MkdirAll": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "a", "b", "c")
			require.NoError(t, fsys.MkdirAll(name, 0o750))
			return []any{stat(t, fsys, filepath.Join(dir, "a")), stat(t, fsys, name)}
		},
		"MkdirAll an existing directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.MkdirAll(filepath.Join(dir, "dir", "sub"), 0o750))
		},

		"Remove a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "file")
			require.NoError(t, fsys.Remove(name))
			_, err := fsys.Stat(name)
			return describe(t, dir, err)
		},
		"Remove an empty directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			require.NoError(t, fsys.Remove(filepath.Join(dir, "empty", ".keep")))
			require.NoError(t, fsys.Remove(filepath.Join(dir, "empty")))
			return entries(t, fsys, dir, nil)
		},
		"Remove a missing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Remove(filepath.Join(dir, "missing")))
		},

		"RemoveAll a tree": func(t *testing.T, fsys conformanceFS, dir string) any {
			require.NoError(t, fsys.RemoveAll(filepath.Join(dir, "dir")))
			return entries(t, fsys, dir, nil)
		},
		"RemoveAll a missing name": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.RemoveAll(filepath.Join(dir, "missing")))
		},

		"Rename a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			require.NoError(t, fsys.Rename(filepath.Join(dir, "dir", "file"), filepath.Join(dir, "moved")))
			return []any{read(t, fsys, filepath.Join(dir, "moved")), entries(t, fsys, filepath.Join(dir, "dir"), nil)}
		},
		"Rename a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			require.NoError(t, fsys.Rename(filepath.Join(dir, "dir"), filepath.Join(dir, "moved")))
			return read(t, fsys, filepath.Join(dir, "moved", "sub", "leaf"))
		},
		"Rename a missing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Rename(filepath.Join(dir, "missing"), filepath.Join(dir, "moved")))
		},

		"Stat a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			return stat(t, fsys, filepath.Join(dir, "dir", "file"))
		},
		"Stat a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			return stat(t, fsys, filepath.Join(dir, "dir", "sub"))
		},
		"Stat a missing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.Stat(filepath.Join(dir, "missing"))
			return describe(t, dir, err)
		},
		"Stat under a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.Stat(filepath.Join(dir, "dir", "file", "x"))
			return describe(t, dir, err)
		},

		"ReadDir": func(t *testing.T, fsys conformanceFS, dir string) any {
			return entries(t, fsys, filepath.Join(dir, "dir"), nil)
		},
		"ReadDir a missing directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.ReadDir(filepath.Join(dir, "missing"))
			return describe(t, dir, err)
		},
		"ReadDir a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.ReadDir(filepath.Join(dir, "dir", "file"))
			return describe(t, dir, err)
		},

		"DirFS walks a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			var walked []string
			err := fs.WalkDir(fsys.DirFS(filepath.Join(dir, "dir")), ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				walked = append(walked, fmt.Sprintf("%s %v", path, d.IsDir()))
				return nil
			})
			require.NoError(t, err)
			return walked
		},
		"DirFS reads a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			b, err := fs.ReadFile(fsys.DirFS(filepath.Join(dir, "dir")), "sub/leaf")
			require.NoError(t, err)
			return string(b)
		},
		"DirFS opens a missing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.DirFS(filepath.Join(dir, "dir")).Open("missing")
			return describe(t, dir, err)
		},
	}

	for name, scenario := range testCases {
		t.Run(name, func(t *testing.T) {
			var want, got any
			t.Run("host.OSFS", func(t *testing.T) {
				dir := t.TempDir()
				for name, data := range conformanceFiles {
					p := filepath.Join(dir, filepath.FromSlash(name))
					require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
					require.NoError(t, os.WriteFile(p, data, 0o644))
				}
				want = scenario(t, host.OSFS{}, dir)
			})
			t.Run("hosttest.FS", func(t *testing.T) {
				h := hosttest.New(t, hosttest.WithFiles(conformanceFiles))
				got = scenario(t, h.FS(), h.Path("."))
			})
			require.Equal(t, want, got, "*hosttest.FS observes what host.OSFS does")
		})
	}
}

// read returns the contents of the named file, opened through fsys.
func read(t *testing.T, fsys conformanceFS, name string) string {
	t.Helper()
	got, err := readAll(fsys, name)
	require.NoError(t, err)
	return got
}

// info is what the table compares of an os.FileInfo.
type info struct {
	Name  string
	Size  int64 // only for a regular file, since a directory's varies
	Mode  fs.FileMode
	IsDir bool
}

func stat(t *testing.T, fsys conformanceFS, name string) info {
	t.Helper()
	fi, err := fsys.Stat(name)
	require.NoError(t, err)
	i := info{Name: fi.Name(), Mode: fi.Mode(), IsDir: fi.IsDir()}
	if fi.Mode().IsRegular() {
		i.Size = fi.Size()
	}
	return i
}

// entry is what the table compares of an os.DirEntry.
type entry struct {
	Name  string
	IsDir bool
	Type  fs.FileMode
}

// entries returns the entries of the directory name in the order ReadDir
// returns them, after checking that it is sorted by filename. rename, if
// not nil, replaces each name, for names CreateTemp picked at random.
func entries(t *testing.T, fsys conformanceFS, name string, rename func(string) string) []entry {
	t.Helper()
	des, err := fsys.ReadDir(name)
	require.NoError(t, err)
	require.True(t, slices.IsSortedFunc(des, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) }), "sorted by filename")
	got := make([]entry, 0, len(des))
	for _, de := range des {
		n := de.Name()
		if rename != nil {
			n = rename(n)
		}
		got = append(got, entry{Name: n, IsDir: de.IsDir(), Type: de.Type()})
	}
	return got
}

// newEntry checks that name, which CreateTemp or MkdirTemp returned, is an
// absolute path naming an entry of dir that matches prefix and suffix, and
// returns what is the same on both filesystems: the entries of dir, with
// name's replaced by the pattern.
func newEntry(t *testing.T, fsys conformanceFS, dir, name, prefix, suffix string) []entry {
	t.Helper()
	require.True(t, filepath.IsAbs(name), name)
	require.Equal(t, dir, filepath.Dir(name), "a new entry in the directory passed")
	base := filepath.Base(name)
	require.True(t, strings.HasPrefix(base, prefix) && strings.HasSuffix(base, suffix), base)
	require.Greater(t, len(base), len(prefix)+len(suffix), "a random part between prefix and suffix")
	return entries(t, fsys, dir, func(n string) string {
		if n == base {
			return prefix + "*" + suffix
		}
		return n
	})
}

// tempPath returns a function that replaces a path naming a random entry
// of dir, matching prefix and suffix, by the pattern, as CreateTemp and
// MkdirTemp name it in their errors.
func tempPath(dir, prefix, suffix string) func(string) string {
	return func(path string) string {
		base := filepath.Base(path)
		if filepath.Dir(path) == dir && strings.HasPrefix(base, prefix) && strings.HasSuffix(base, suffix) {
			return filepath.Join(dir, prefix+"*"+suffix)
		}
		return path
	}
}

// failure is what the table compares of an error.
type failure struct {
	Type     string
	Path     string // relative to the scenario's directory
	Old, New string // relative to the scenario's directory, for an *os.LinkError
	Is       []string
}

// classes are the errors.Is classes the table compares.
var classes = []struct {
	name string
	err  error
}{
	{"fs.ErrNotExist", fs.ErrNotExist},
	{"fs.ErrExist", fs.ErrExist},
	{"fs.ErrPermission", fs.ErrPermission},
	{"fs.ErrInvalid", fs.ErrInvalid},
	{"syscall.ENOTDIR", syscall.ENOTDIR},
	{"syscall.EISDIR", syscall.EISDIR},
	{"syscall.ENOTEMPTY", syscall.ENOTEMPTY},
}

// describe returns what the table compares of err, which is nil for no
// error. Paths are made relative to dir, after each is passed through
// rename, if given, for the random names of CreateTemp and MkdirTemp.
func describe(t *testing.T, dir string, err error, rename ...func(string) string) *failure {
	t.Helper()
	if err == nil {
		return nil
	}
	relTo := func(path string) string {
		for _, r := range rename {
			path = r(path)
		}
		if !filepath.IsAbs(path) {
			return path
		}
		rel, err := filepath.Rel(dir, path)
		require.NoError(t, err)
		return filepath.ToSlash(rel)
	}

	f := &failure{Type: fmt.Sprintf("%T", err)}
	var pe *fs.PathError
	var le *os.LinkError
	switch {
	case errors.As(err, &le):
		f.Old, f.New = relTo(le.Old), relTo(le.New)
	case errors.As(err, &pe):
		f.Path = relTo(pe.Path)
	}
	for _, c := range classes {
		if errors.Is(err, c.err) {
			f.Is = append(f.Is, c.name)
		}
	}
	return f
}
