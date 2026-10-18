// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package host_test

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host"
)

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(name, []byte(content), 0o600))
}

func readAll(t *testing.T, f *os.File) string {
	t.Helper()
	b, err := io.ReadAll(f)
	require.NoError(t, err)
	return string(b)
}

func closeFile(t *testing.T, f *os.File) {
	t.Helper()
	require.NoError(t, f.Close())
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(b)
}

// requirePathError requires that err is a *fs.PathError naming path and
// wrapping target.
func requirePathError(t *testing.T, err error, path string, target error) {
	t.Helper()
	require.IsType(t, &fs.PathError{}, err)
	require.Equal(t, path, err.(*fs.PathError).Path)
	require.ErrorIs(t, err, target)
}

// requireLinkError requires that err is a *os.LinkError naming oldname and
// newname and wrapping target.
func requireLinkError(t *testing.T, err error, oldname, newname string, target error) {
	t.Helper()
	require.IsType(t, &os.LinkError{}, err)
	le := err.(*os.LinkError)
	require.Equal(t, oldname, le.Old)
	require.Equal(t, newname, le.New)
	require.ErrorIs(t, err, target)
}

// owner returns the user and group that own the file info describes.
func owner(t *testing.T, info os.FileInfo) (uid, gid int) {
	t.Helper()
	st, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok, "Sys is %T, want *syscall.Stat_t", info.Sys())
	return int(st.Uid), int(st.Gid)
}

func TestOSFS(t *testing.T) {
	var fsys host.OSFS

	t.Run("Open", func(t *testing.T) {
		t.Run("reads an existing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")
			writeFile(t, name, "hello")

			f, err := fsys.Open(name)
			require.NoError(t, err)
			defer closeFile(t, f)

			require.Equal(t, name, f.Name())
			require.Equal(t, "hello", readAll(t, f))
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing")

			_, err := fsys.Open(name)
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("OpenFile", func(t *testing.T) {
		t.Run("creates and writes a file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "b.txt")

			f, err := fsys.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			require.NoError(t, err)
			_, err = io.WriteString(f, "written")
			require.NoError(t, err)
			closeFile(t, f)

			require.Equal(t, "written", readFile(t, name))
		})

		t.Run("with O_EXCL fails for an existing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "b.txt")
			writeFile(t, name, "")

			_, err := fsys.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			requirePathError(t, err, name, fs.ErrExist)
		})
	})

	t.Run("Create", func(t *testing.T) {
		t.Run("truncates an existing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")
			writeFile(t, name, "old content")

			f, err := fsys.Create(name)
			require.NoError(t, err)
			_, err = io.WriteString(f, "new")
			require.NoError(t, err)
			closeFile(t, f)

			require.Equal(t, "new", readFile(t, name))
		})

		t.Run("fails in a missing directory", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing", "a.txt")

			_, err := fsys.Create(name)
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("CreateTemp", func(t *testing.T) {
		t.Run("creates a new file in dir", func(t *testing.T) {
			dir := t.TempDir()

			f, err := fsys.CreateTemp(dir, "tmp-*.txt")
			require.NoError(t, err)
			defer closeFile(t, f)

			require.Equal(t, dir, filepath.Dir(f.Name()))
			base := filepath.Base(f.Name())
			require.True(t, strings.HasPrefix(base, "tmp-"), "name %q does not match tmp-*.txt", base)
			require.True(t, strings.HasSuffix(base, ".txt"), "name %q does not match tmp-*.txt", base)
			_, err = os.Stat(f.Name())
			require.NoError(t, err)
		})

		t.Run("fails in a missing directory", func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "missing")

			_, err := fsys.CreateTemp(dir, "tmp-*.txt")
			require.IsType(t, &fs.PathError{}, err)
			require.ErrorIs(t, err, fs.ErrNotExist)
			// os.CreateTemp names the file it tried to create.
			name := err.(*fs.PathError).Path
			require.Equal(t, dir, filepath.Dir(name))
			base := filepath.Base(name)
			require.True(t, strings.HasPrefix(base, "tmp-"), "name %q does not match tmp-*.txt", base)
			require.True(t, strings.HasSuffix(base, ".txt"), "name %q does not match tmp-*.txt", base)
		})
	})

	t.Run("OpenRoot", func(t *testing.T) {
		t.Run("opens a directory as a root", func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "a.txt"), "in the root")

			root, err := fsys.OpenRoot(dir)
			require.NoError(t, err)
			defer func() { require.NoError(t, root.Close()) }()

			require.Equal(t, dir, root.Name())
			b, err := root.ReadFile("a.txt")
			require.NoError(t, err)
			require.Equal(t, "in the root", string(b))
		})

		t.Run("fails for a missing directory", func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "missing")

			_, err := fsys.OpenRoot(dir)
			requirePathError(t, err, dir, fs.ErrNotExist)
		})
	})

	t.Run("OpenInRoot", func(t *testing.T) {
		t.Run("opens a file in dir", func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "a.txt"), "in the root")

			f, err := fsys.OpenInRoot(dir, "a.txt")
			require.NoError(t, err)
			defer closeFile(t, f)

			require.Equal(t, "in the root", readAll(t, f))
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			dir := t.TempDir()

			_, err := fsys.OpenInRoot(dir, "missing")
			requirePathError(t, err, "missing", fs.ErrNotExist)
		})

		t.Run("fails for a name that escapes dir", func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "root")
			require.NoError(t, os.Mkdir(dir, 0o700))
			writeFile(t, filepath.Join(parent, "outside.txt"), "")

			name := filepath.Join("..", "outside.txt")
			_, err := fsys.OpenInRoot(dir, name)
			require.IsType(t, &fs.PathError{}, err)
			require.Equal(t, name, err.(*fs.PathError).Path)
		})
	})

	t.Run("ReadFile", func(t *testing.T) {
		t.Run("reads a whole file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")
			writeFile(t, name, "whole")

			b, err := fsys.ReadFile(name)
			require.NoError(t, err)
			require.Equal(t, "whole", string(b))
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing")

			_, err := fsys.ReadFile(name)
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("WriteFile", func(t *testing.T) {
		t.Run("creates a file with perm", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")

			require.NoError(t, fsys.WriteFile(name, []byte("written"), 0o600))

			require.Equal(t, "written", readFile(t, name))
			info, err := os.Stat(name)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		})

		t.Run("fails in a missing directory", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing", "a.txt")

			err := fsys.WriteFile(name, []byte("written"), 0o600)
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("Truncate", func(t *testing.T) {
		t.Run("changes the size of a file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")
			writeFile(t, name, "12345")

			require.NoError(t, fsys.Truncate(name, 2))

			require.Equal(t, "12", readFile(t, name))
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing")

			err := fsys.Truncate(name, 2)
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("CopyFS", func(t *testing.T) {
		src := fstest.MapFS{
			"a.txt":     {Data: []byte("top"), Mode: 0o600},
			"b/c/d.txt": {Data: []byte("deep"), Mode: 0o600},
		}

		t.Run("copies a filesystem into dir", func(t *testing.T) {
			dir := t.TempDir()

			require.NoError(t, fsys.CopyFS(dir, src))

			require.Equal(t, "top", readFile(t, filepath.Join(dir, "a.txt")))
			require.Equal(t, "deep", readFile(t, filepath.Join(dir, "b", "c", "d.txt")))
		})

		t.Run("fails when a file already exists", func(t *testing.T) {
			dir := t.TempDir()
			name := filepath.Join(dir, "a.txt")
			writeFile(t, name, "")

			err := fsys.CopyFS(dir, src)
			requirePathError(t, err, name, fs.ErrExist)
		})
	})

	t.Run("Mkdir", func(t *testing.T) {
		t.Run("creates a directory", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "sub")

			require.NoError(t, fsys.Mkdir(name, 0o700))

			info, err := os.Stat(name)
			require.NoError(t, err)
			require.True(t, info.IsDir(), "%q is not a directory", name)
		})

		t.Run("fails for an existing directory", func(t *testing.T) {
			dir := t.TempDir()

			err := fsys.Mkdir(dir, 0o700)
			requirePathError(t, err, dir, fs.ErrExist)
		})
	})

	t.Run("MkdirAll", func(t *testing.T) {
		t.Run("creates every missing parent", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a", "b", "c")

			require.NoError(t, fsys.MkdirAll(name, 0o700))

			info, err := os.Stat(name)
			require.NoError(t, err)
			require.True(t, info.IsDir(), "%q is not a directory", name)
		})

		t.Run("fails for an existing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")
			writeFile(t, name, "")

			err := fsys.MkdirAll(name, 0o700)
			requirePathError(t, err, name, syscall.ENOTDIR)
		})
	})

	t.Run("MkdirTemp", func(t *testing.T) {
		t.Run("creates a new directory in dir", func(t *testing.T) {
			dir := t.TempDir()

			name, err := fsys.MkdirTemp(dir, "tmp-")
			require.NoError(t, err)

			require.Equal(t, dir, filepath.Dir(name))
			require.True(t, strings.HasPrefix(filepath.Base(name), "tmp-"), "name %q does not match tmp-*", name)
			info, err := os.Stat(name)
			require.NoError(t, err)
			require.True(t, info.IsDir(), "%q is not a directory", name)
		})

		t.Run("fails in a missing directory", func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "missing")

			_, err := fsys.MkdirTemp(dir, "tmp-")
			requirePathError(t, err, dir, fs.ErrNotExist)
		})
	})

	t.Run("ReadDir", func(t *testing.T) {
		t.Run("lists a directory sorted by name", func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "b.txt"), "")
			writeFile(t, filepath.Join(dir, "a.txt"), "")
			require.NoError(t, os.Mkdir(filepath.Join(dir, "c"), 0o700))

			entries, err := fsys.ReadDir(dir)
			require.NoError(t, err)

			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			require.Equal(t, []string{"a.txt", "b.txt", "c"}, names)
			require.True(t, entries[2].IsDir(), "entry %q is not a directory", entries[2].Name())
		})

		t.Run("fails for a missing directory", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing")

			_, err := fsys.ReadDir(name)
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("Remove", func(t *testing.T) {
		t.Run("removes a file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")
			writeFile(t, name, "")

			require.NoError(t, fsys.Remove(name))

			_, err := os.Stat(name)
			require.ErrorIs(t, err, fs.ErrNotExist)
		})

		t.Run("fails on a directory that is not empty", func(t *testing.T) {
			sub := filepath.Join(t.TempDir(), "sub")
			require.NoError(t, os.Mkdir(sub, 0o700))
			writeFile(t, filepath.Join(sub, "a.txt"), "")

			err := fsys.Remove(sub)
			requirePathError(t, err, sub, syscall.ENOTEMPTY)
		})
	})

	t.Run("RemoveAll", func(t *testing.T) {
		t.Run("removes a tree", func(t *testing.T) {
			sub := filepath.Join(t.TempDir(), "sub")
			require.NoError(t, os.MkdirAll(filepath.Join(sub, "nested"), 0o700))
			writeFile(t, filepath.Join(sub, "nested", "a.txt"), "")

			require.NoError(t, fsys.RemoveAll(sub))

			_, err := os.Stat(sub)
			require.ErrorIs(t, err, fs.ErrNotExist)
		})

		t.Run("fails for a name that ends in a dot", func(t *testing.T) {
			name := t.TempDir() + string(filepath.Separator) + "."

			err := fsys.RemoveAll(name)
			requirePathError(t, err, name, syscall.EINVAL)
		})
	})

	t.Run("Rename", func(t *testing.T) {
		t.Run("moves a file", func(t *testing.T) {
			dir := t.TempDir()
			oldname := filepath.Join(dir, "old.txt")
			newname := filepath.Join(dir, "new.txt")
			writeFile(t, oldname, "moved")

			require.NoError(t, fsys.Rename(oldname, newname))

			_, err := os.Stat(oldname)
			require.ErrorIs(t, err, fs.ErrNotExist)
			require.Equal(t, "moved", readFile(t, newname))
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			dir := t.TempDir()
			oldname := filepath.Join(dir, "missing")
			newname := filepath.Join(dir, "new.txt")

			err := fsys.Rename(oldname, newname)
			requireLinkError(t, err, oldname, newname, fs.ErrNotExist)
		})
	})

	t.Run("Link", func(t *testing.T) {
		t.Run("makes a hard link", func(t *testing.T) {
			dir := t.TempDir()
			oldname := filepath.Join(dir, "a.txt")
			newname := filepath.Join(dir, "b.txt")
			writeFile(t, oldname, "shared")

			require.NoError(t, fsys.Link(oldname, newname))

			oldInfo, err := os.Stat(oldname)
			require.NoError(t, err)
			newInfo, err := os.Stat(newname)
			require.NoError(t, err)
			require.True(t, os.SameFile(oldInfo, newInfo), "%q and %q are not the same file", oldname, newname)
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			dir := t.TempDir()
			oldname := filepath.Join(dir, "missing")
			newname := filepath.Join(dir, "b.txt")

			err := fsys.Link(oldname, newname)
			requireLinkError(t, err, oldname, newname, fs.ErrNotExist)
		})
	})

	t.Run("Symlink", func(t *testing.T) {
		t.Run("makes a symbolic link", func(t *testing.T) {
			dir := t.TempDir()
			oldname := filepath.Join(dir, "a.txt")
			newname := filepath.Join(dir, "link")
			writeFile(t, oldname, "through the link")

			require.NoError(t, fsys.Symlink(oldname, newname))

			target, err := os.Readlink(newname)
			require.NoError(t, err)
			require.Equal(t, oldname, target)
			require.Equal(t, "through the link", readFile(t, newname))
		})

		t.Run("fails when newname exists", func(t *testing.T) {
			dir := t.TempDir()
			oldname := filepath.Join(dir, "a.txt")
			newname := filepath.Join(dir, "b.txt")
			writeFile(t, oldname, "")
			writeFile(t, newname, "")

			err := fsys.Symlink(oldname, newname)
			requireLinkError(t, err, oldname, newname, fs.ErrExist)
		})
	})

	t.Run("Readlink", func(t *testing.T) {
		t.Run("returns the target of a symbolic link", func(t *testing.T) {
			dir := t.TempDir()
			link := filepath.Join(dir, "link")
			require.NoError(t, os.Symlink("target.txt", link))

			target, err := fsys.Readlink(link)
			require.NoError(t, err)
			require.Equal(t, "target.txt", target)
		})

		t.Run("fails for a file that is not a link", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")
			writeFile(t, name, "")

			_, err := fsys.Readlink(name)
			requirePathError(t, err, name, syscall.EINVAL)
		})
	})

	t.Run("Stat", func(t *testing.T) {
		t.Run("describes a file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")
			writeFile(t, name, "12345")

			info, err := fsys.Stat(name)
			require.NoError(t, err)

			require.Equal(t, "a.txt", info.Name())
			require.Equal(t, int64(5), info.Size())
			require.True(t, info.Mode().IsRegular(), "Mode is %v, want a regular file", info.Mode())
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing")

			_, err := fsys.Stat(name)
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("Lstat", func(t *testing.T) {
		t.Run("describes a symbolic link, not its target", func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "a.txt")
			link := filepath.Join(dir, "link")
			writeFile(t, target, "12345")
			require.NoError(t, os.Symlink(target, link))

			info, err := fsys.Lstat(link)
			require.NoError(t, err)

			require.Equal(t, "link", info.Name())
			require.Equal(t, fs.ModeSymlink, info.Mode().Type())
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing")

			_, err := fsys.Lstat(name)
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("Chmod", func(t *testing.T) {
		t.Run("changes the mode of a file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")
			writeFile(t, name, "")

			require.NoError(t, fsys.Chmod(name, 0o400))

			info, err := os.Stat(name)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0o400), info.Mode().Perm())
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing")

			err := fsys.Chmod(name, 0o400)
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("Chown", func(t *testing.T) {
		t.Run("sets the owner of a file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")
			writeFile(t, name, "")

			require.NoError(t, fsys.Chown(name, os.Getuid(), os.Getgid()))

			info, err := os.Stat(name)
			require.NoError(t, err)
			uid, gid := owner(t, info)
			require.Equal(t, os.Getuid(), uid)
			require.Equal(t, os.Getgid(), gid)
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing")

			err := fsys.Chown(name, os.Getuid(), os.Getgid())
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("Lchown", func(t *testing.T) {
		t.Run("sets the owner of a symbolic link", func(t *testing.T) {
			link := filepath.Join(t.TempDir(), "link")
			require.NoError(t, os.Symlink("missing-target", link))

			require.NoError(t, fsys.Lchown(link, os.Getuid(), os.Getgid()))

			info, err := os.Lstat(link)
			require.NoError(t, err)
			uid, gid := owner(t, info)
			require.Equal(t, os.Getuid(), uid)
			require.Equal(t, os.Getgid(), gid)
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing")

			err := fsys.Lchown(name, os.Getuid(), os.Getgid())
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("Chtimes", func(t *testing.T) {
		t.Run("changes the modification time of a file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "a.txt")
			writeFile(t, name, "")
			mtime := time.Date(2001, time.February, 3, 4, 5, 6, 0, time.UTC)

			require.NoError(t, fsys.Chtimes(name, mtime, mtime))

			info, err := os.Stat(name)
			require.NoError(t, err)
			require.True(t, mtime.Equal(info.ModTime()), "ModTime is %v, want %v", info.ModTime(), mtime)
		})

		t.Run("fails for a missing file", func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "missing")
			mtime := time.Date(2001, time.February, 3, 4, 5, 6, 0, time.UTC)

			err := fsys.Chtimes(name, mtime, mtime)
			requirePathError(t, err, name, fs.ErrNotExist)
		})
	})

	t.Run("Getwd", func(t *testing.T) {
		t.Run("returns what os.Getwd returns", func(t *testing.T) {
			want, err := os.Getwd()
			require.NoError(t, err)

			got, err := fsys.Getwd()
			require.NoError(t, err)
			require.Equal(t, want, got)
			require.True(t, filepath.IsAbs(got), "%q is not absolute", got)
		})
	})

	t.Run("DirFS", func(t *testing.T) {
		t.Run("can be walked with fs.WalkDir", func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b"), 0o700))
			writeFile(t, filepath.Join(dir, "top.txt"), "")
			writeFile(t, filepath.Join(dir, "a", "b", "deep.txt"), "deep")

			dirFS := fsys.DirFS(dir)

			var walked []string
			err := fs.WalkDir(dirFS, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				walked = append(walked, path)
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, []string{".", "a", "a/b", "a/b/deep.txt", "top.txt"}, walked)

			b, err := fs.ReadFile(dirFS, "a/b/deep.txt")
			require.NoError(t, err)
			require.Equal(t, "deep", string(b))
		})

		t.Run("fails like os.DirFS for a missing file", func(t *testing.T) {
			dirFS := fsys.DirFS(t.TempDir())

			_, err := fs.ReadFile(dirFS, "missing")
			requirePathError(t, err, "missing", fs.ErrNotExist)
		})
	})
}

// An os.Root refuses to follow an absolute symbolic link wherever it is
// rooted, "/" included, which is why OSFS is built on plain os calls. The
// link here points at an absolute path inside the test's own directory, so
// the test touches no other file.
func TestOSFS_follows_an_absolute_symlink(t *testing.T) {
	var fsys host.OSFS

	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	writeFile(t, target, "through the link")
	require.True(t, filepath.IsAbs(target), "target %q is not absolute", target)
	require.NoError(t, os.Symlink(target, link))

	t.Run("os.Root refuses it", func(t *testing.T) {
		root, err := os.OpenRoot(dir)
		require.NoError(t, err)
		defer func() { require.NoError(t, root.Close()) }()

		f, err := root.Open("link.txt")
		if err == nil {
			closeFile(t, f)
		}
		require.Error(t, err, "os.Root followed an absolute symlink")
	})

	t.Run("Open follows it", func(t *testing.T) {
		f, err := fsys.Open(link)
		require.NoError(t, err)
		defer closeFile(t, f)

		require.Equal(t, "through the link", readAll(t, f))
	})

	t.Run("Stat follows it", func(t *testing.T) {
		info, err := fsys.Stat(link)
		require.NoError(t, err)
		require.True(t, info.Mode().IsRegular(), "Mode is %v, want the regular file the link points at", info.Mode())
	})
}
