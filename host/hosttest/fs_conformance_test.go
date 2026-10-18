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
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host"
	"github.com/z5labs/bedrock/host/hosttest"
)

// conformanceFS lists every method of host.OSFS, which the FS conformance
// table checks *hosttest.FS has, with the same results as host.OSFS. Getwd
// is the one whose results it does not compare.
type conformanceFS interface {
	Open(name string) (*os.File, error)
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	Create(name string) (*os.File, error)
	CreateTemp(dir, pattern string) (*os.File, error)
	OpenRoot(name string) (*os.Root, error)
	OpenInRoot(dir, name string) (*os.File, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	Truncate(name string, size int64) error
	CopyFS(dir string, fsys fs.FS) error
	Mkdir(name string, perm os.FileMode) error
	MkdirAll(name string, perm os.FileMode) error
	MkdirTemp(dir, pattern string) (string, error)
	ReadDir(name string) ([]os.DirEntry, error)
	Remove(name string) error
	RemoveAll(name string) error
	Rename(oldname, newname string) error
	Link(oldname, newname string) error
	Symlink(oldname, newname string) error
	Readlink(name string) (string, error)
	Stat(name string) (os.FileInfo, error)
	Lstat(name string) (os.FileInfo, error)
	Chmod(name string, mode os.FileMode) error
	Chown(name string, uid, gid int) error
	Lchown(name string, uid, gid int) error
	Chtimes(name string, atime, mtime time.Time) error
	Getwd() (string, error)
	DirFS(dir string) fs.FS
}

// Both filesystems have every method conformanceFS lists, so this stops
// compiling if *hosttest.FS lacks one of host.OSFS's.
var (
	_ conformanceFS = host.OSFS{}
	_ conformanceFS = (*hosttest.FS)(nil)
)

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
		"MkdirAll an existing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.MkdirAll(filepath.Join(dir, "dir", "file"), 0o750))
		},
		"MkdirAll a symbolic link to a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "link")
			require.NoError(t, fsys.Symlink("dir", name))
			return describe(t, dir, fsys.MkdirAll(name, 0o750))
		},
		"MkdirAll a dangling symbolic link": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "link")
			require.NoError(t, fsys.Symlink("missing", name))
			return describe(t, dir, fsys.MkdirAll(name, 0o750))
		},
		"MkdirAll under a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.MkdirAll(filepath.Join(dir, "dir", "file", "a", "b"), 0o750))
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

		"Create a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "new")
			f, err := fsys.Create(name)
			require.NoError(t, err)
			_, err = io.WriteString(f, "new")
			require.NoError(t, err)
			require.NoError(t, f.Close())
			return []any{read(t, fsys, name), stat(t, fsys, name)}
		},
		"Create truncates an existing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "file")
			f, err := fsys.Create(name)
			require.NoError(t, err)
			require.NoError(t, f.Close())
			return read(t, fsys, name)
		},
		"Create in a missing directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.Create(filepath.Join(dir, "missing", "new"))
			return describe(t, dir, err)
		},

		"OpenRoot a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			root, err := fsys.OpenRoot(filepath.Join(dir, "dir"))
			require.NoError(t, err)
			defer func() { require.NoError(t, root.Close()) }()
			b, err := root.ReadFile(filepath.Join("sub", "leaf"))
			require.NoError(t, err)
			_, escaped := root.Open(filepath.Join("..", "empty", ".keep"))
			return []any{string(b), describe(t, dir, escaped)}
		},
		"OpenRoot a missing directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.OpenRoot(filepath.Join(dir, "missing"))
			return describe(t, dir, err)
		},

		"OpenInRoot a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			f, err := fsys.OpenInRoot(filepath.Join(dir, "dir"), filepath.Join("sub", "leaf"))
			require.NoError(t, err)
			defer func() { require.NoError(t, f.Close()) }()
			b, err := io.ReadAll(f)
			require.NoError(t, err)
			return string(b)
		},
		"OpenInRoot a name that leaves dir": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.OpenInRoot(filepath.Join(dir, "dir", "sub"), filepath.Join("..", "file"))
			return describe(t, dir, err)
		},
		"OpenInRoot a missing dir": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.OpenInRoot(filepath.Join(dir, "missing"), "file")
			return describe(t, dir, err)
		},
		"OpenInRoot a missing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.OpenInRoot(filepath.Join(dir, "dir"), "missing")
			return describe(t, dir, err)
		},

		"ReadFile": func(t *testing.T, fsys conformanceFS, dir string) any {
			b, err := fsys.ReadFile(filepath.Join(dir, "dir", "sub", "leaf"))
			require.NoError(t, err)
			return string(b)
		},
		"ReadFile a missing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.ReadFile(filepath.Join(dir, "missing"))
			return describe(t, dir, err)
		},
		"ReadFile a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.ReadFile(filepath.Join(dir, "dir"))
			return describe(t, dir, err)
		},

		"WriteFile creates a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "new")
			require.NoError(t, fsys.WriteFile(name, []byte("new"), 0o600))
			return []any{read(t, fsys, name), stat(t, fsys, name)}
		},
		"WriteFile replaces a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "file")
			require.NoError(t, fsys.WriteFile(name, []byte("replaced"), 0o600))
			return []any{read(t, fsys, name), stat(t, fsys, name)}
		},
		"WriteFile in a missing directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.WriteFile(filepath.Join(dir, "missing", "new"), nil, 0o600))
		},
		"WriteFile a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.WriteFile(filepath.Join(dir, "dir"), nil, 0o600))
		},

		"Truncate shrinks a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "file")
			require.NoError(t, fsys.Truncate(name, 2))
			return read(t, fsys, name)
		},
		"Truncate extends a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "file")
			require.NoError(t, fsys.Truncate(name, 6))
			return []any{read(t, fsys, name), stat(t, fsys, name)}
		},
		"Truncate a missing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Truncate(filepath.Join(dir, "missing"), 0))
		},
		"Truncate a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Truncate(filepath.Join(dir, "dir"), 0))
		},
		"Truncate to a negative size": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Truncate(filepath.Join(dir, "dir", "file"), -1))
		},

		"CopyFS directories, files and relative symbolic links": func(t *testing.T, fsys conformanceFS, dir string) any {
			to := filepath.Join(dir, "copy")
			require.NoError(t, fsys.CopyFS(to, fstest.MapFS{
				"a/b":     {Data: []byte("b"), Mode: 0o640},
				"a/x":     {Data: []byte("x"), Mode: 0o755},
				"a/link":  {Data: []byte("b"), Mode: fs.ModeSymlink},
				"a/up":    {Data: []byte("../c"), Mode: fs.ModeSymlink},
				"c":       {Mode: fs.ModeDir | 0o700},
				"d/e/f/g": {Data: []byte("g")},
			}))
			return tree(t, fsys, to)
		},
		"CopyFS into an existing directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			require.NoError(t, fsys.CopyFS(filepath.Join(dir, "dir"), fstest.MapFS{"sub/new": {Data: []byte("new")}}))
			return tree(t, fsys, filepath.Join(dir, "dir"))
		},
		"CopyFS onto an existing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			err := fsys.CopyFS(filepath.Join(dir, "dir"), fstest.MapFS{"file": {Data: []byte("overwritten")}})
			return []any{describe(t, dir, err), read(t, fsys, filepath.Join(dir, "dir", "file"))}
		},
		"CopyFS a symbolic link onto an existing name": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.CopyFS(filepath.Join(dir, "dir"), fstest.MapFS{"file": {Data: []byte("sub"), Mode: fs.ModeSymlink}}))
		},
		"CopyFS a named pipe": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.CopyFS(filepath.Join(dir, "copy"), fstest.MapFS{"pipe": {Mode: fs.ModeNamedPipe}}))
		},
		"CopyFS under a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.CopyFS(filepath.Join(dir, "dir", "file"), fstest.MapFS{"new": {Data: []byte("new")}}))
		},

		"Link": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "hard")
			require.NoError(t, fsys.Link(filepath.Join(dir, "dir", "file"), name))
			return []any{read(t, fsys, name), stat(t, fsys, name)}
		},
		"Link a missing file": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Link(filepath.Join(dir, "missing"), filepath.Join(dir, "hard")))
		},
		"Link onto an existing name": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Link(filepath.Join(dir, "dir", "file"), filepath.Join(dir, "dir", "sub", "leaf")))
		},

		"Symlink to a relative target": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "link")
			require.NoError(t, fsys.Symlink(filepath.Join("sub", "leaf"), name))
			target, err := fsys.Readlink(name)
			require.NoError(t, err)
			return []any{target, read(t, fsys, name), lstat(t, fsys, name), stat(t, fsys, name)}
		},
		"Symlink to a relative directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "link")
			require.NoError(t, fsys.Symlink("dir", name))
			return []any{read(t, fsys, filepath.Join(name, "sub", "leaf")), entries(t, fsys, name, nil)}
		},
		"Symlink to a missing target": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dangling")
			require.NoError(t, fsys.Symlink("missing", name))
			_, err := fsys.Stat(name)
			return []any{lstat(t, fsys, name), describe(t, dir, err)}
		},
		"Symlink onto an existing name": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Symlink("file", filepath.Join(dir, "dir", "sub")))
		},
		"Symlink in a missing directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Symlink("file", filepath.Join(dir, "missing", "link")))
		},

		"Readlink a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.Readlink(filepath.Join(dir, "dir", "file"))
			return describe(t, dir, err)
		},
		"Readlink a missing name": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.Readlink(filepath.Join(dir, "missing"))
			return describe(t, dir, err)
		},

		"Lstat a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			return lstat(t, fsys, filepath.Join(dir, "dir", "file"))
		},
		"Lstat a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			return lstat(t, fsys, filepath.Join(dir, "dir", "sub"))
		},
		"Lstat a missing name": func(t *testing.T, fsys conformanceFS, dir string) any {
			_, err := fsys.Lstat(filepath.Join(dir, "missing"))
			return describe(t, dir, err)
		},

		"Chmod a file": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "file")
			require.NoError(t, fsys.Chmod(name, 0o600))
			return stat(t, fsys, name)
		},
		"Chmod a directory": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "sub")
			require.NoError(t, fsys.Chmod(name, 0o700))
			return stat(t, fsys, name)
		},
		"Chmod a missing name": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Chmod(filepath.Join(dir, "missing"), 0o600))
		},

		"Chown to the current user": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "file")
			return []any{describe(t, dir, fsys.Chown(name, os.Getuid(), os.Getgid())), stat(t, fsys, name)}
		},
		"Chown a missing name": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Chown(filepath.Join(dir, "missing"), os.Getuid(), os.Getgid()))
		},

		"Lchown a symbolic link to the current user": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "link")
			require.NoError(t, fsys.Symlink("dir", name))
			return []any{describe(t, dir, fsys.Lchown(name, os.Getuid(), os.Getgid())), lstat(t, fsys, name)}
		},
		"Lchown a missing name": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Lchown(filepath.Join(dir, "missing"), os.Getuid(), os.Getgid()))
		},

		"Chtimes": func(t *testing.T, fsys conformanceFS, dir string) any {
			name := filepath.Join(dir, "dir", "file")
			atime := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
			mtime := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
			require.NoError(t, fsys.Chtimes(name, atime, mtime))
			fi, err := fsys.Stat(name)
			require.NoError(t, err)
			return []any{accessTime(fi), fi.ModTime().UTC()}
		},
		"Chtimes a missing name": func(t *testing.T, fsys conformanceFS, dir string) any {
			return describe(t, dir, fsys.Chtimes(filepath.Join(dir, "missing"), time.Time{}, time.Time{}))
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

// linkScenario is one row of the FS conformance table's one exception: a
// symbolic link to an absolute target. It runs on fsys, whose directory is
// dir, and returns the target the link was made with, the name of the
// link, and the error of the call that made it.
type linkScenario func(t *testing.T, fsys conformanceFS, dir string) (target, name string, err error)

// TestConformance_FS_absolute_symbolic_links is the exception to
// TestConformance_FS: host.OSFS makes a symbolic link to an absolute
// target, and *hosttest.FS refuses to, with an *os.LinkError naming both
// paths whose Err is errors.ErrUnsupported, and creates nothing. Its
// os.Root would never follow such a link, even to a file inside its
// directory.
func TestConformance_FS_absolute_symbolic_links(t *testing.T) {
	testCases := map[string]linkScenario{
		"Symlink to a target inside the directory": func(t *testing.T, fsys conformanceFS, dir string) (string, string, error) {
			target, name := filepath.Join(dir, "dir", "file"), filepath.Join(dir, "link")
			return target, name, fsys.Symlink(target, name)
		},
		"Symlink to a target outside the directory": func(t *testing.T, fsys conformanceFS, dir string) (string, string, error) {
			target, name := filepath.Join(filepath.Dir(dir), "outside"), filepath.Join(dir, "dir", "link")
			return target, name, fsys.Symlink(target, name)
		},
		"CopyFS of a symbolic link to an absolute target": func(t *testing.T, fsys conformanceFS, dir string) (string, string, error) {
			target, to := filepath.Join(dir, "dir", "file"), filepath.Join(dir, "copy")
			err := fsys.CopyFS(to, fstest.MapFS{"link": {Data: []byte(target), Mode: fs.ModeSymlink}})
			return target, filepath.Join(to, "link"), err
		},
	}

	for name, scenario := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Run("host.OSFS", func(t *testing.T) {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "dir"), 0o755))
				target, link, err := scenario(t, host.OSFS{}, dir)
				require.NoError(t, err)
				got, err := os.Readlink(link)
				require.NoError(t, err)
				require.Equal(t, target, got)
			})
			t.Run("hosttest.FS", func(t *testing.T) {
				h := hosttest.New(t, hosttest.WithFiles(conformanceFiles))
				target, link, err := scenario(t, h.FS(), h.Path("."))
				var le *os.LinkError
				require.ErrorAs(t, err, &le)
				require.Equal(t, "symlink", le.Op)
				require.Equal(t, target, le.Old)
				require.Equal(t, link, le.New)
				require.ErrorIs(t, err, errors.ErrUnsupported)
				_, err = os.Lstat(link)
				require.ErrorIs(t, err, fs.ErrNotExist, "nothing was created")
			})
		})
	}
}

// Getwd is the one method of host.OSFS the table does not compare: OSFS's
// is the process's working directory, and *hosttest.FS's is its Host's
// directory.
func TestConformance_FS_Getwd(t *testing.T) {
	t.Run("host.OSFS", func(t *testing.T) {
		want, err := os.Getwd()
		require.NoError(t, err)
		got, err := host.OSFS{}.Getwd()
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
	t.Run("hosttest.FS", func(t *testing.T) {
		h := hosttest.New(t)
		got, err := h.FS().Getwd()
		require.NoError(t, err)
		require.True(t, filepath.IsAbs(got), got)
		require.Equal(t, h.Path("."), got)
	})
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
	return infoOf(fi)
}

// lstat is stat, with Lstat.
func lstat(t *testing.T, fsys conformanceFS, name string) info {
	t.Helper()
	fi, err := fsys.Lstat(name)
	require.NoError(t, err)
	return infoOf(fi)
}

func infoOf(fi os.FileInfo) info {
	i := info{Name: fi.Name(), Mode: fi.Mode(), IsDir: fi.IsDir()}
	if fi.Mode().IsRegular() {
		i.Size = fi.Size()
	}
	return i
}

// tree returns every name under dir, without following symbolic links,
// with what Lstat says of it, and the contents of each file or the target
// of each symbolic link.
func tree(t *testing.T, fsys conformanceFS, dir string) []string {
	t.Helper()
	var got []string
	var walk func(name string)
	walk = func(name string) {
		for _, e := range entries(t, fsys, name, nil) {
			p := filepath.Join(name, e.Name)
			rel, err := filepath.Rel(dir, p)
			require.NoError(t, err)
			i := lstat(t, fsys, p)
			line := fmt.Sprintf("%s %v %d", filepath.ToSlash(rel), i.Mode, i.Size)
			switch {
			case i.IsDir:
				got = append(got, line)
				walk(p)
			case i.Mode&fs.ModeSymlink != 0:
				target, err := fsys.Readlink(p)
				require.NoError(t, err)
				got = append(got, line+" -> "+target)
			default:
				got = append(got, line+" "+read(t, fsys, p))
			}
		}
	}
	walk(dir)
	return got
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
