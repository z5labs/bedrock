// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package host

import (
	"io/fs"
	"os"
	"time"
)

// OSFS is the filesystem of the real machine. It has no fields, and its
// zero value is ready to use:
//
//	var fsys host.OSFS
//
// It has one method for every function of package os that names a file,
// except those that read or change process-wide state, and each method is
// the os function of the same name. CreateTemp and MkdirTemp with
// dir == "" use os.TempDir.
//
// Names are OS paths, and a relative name is relative to Getwd. Files are
// *os.File, so a file opened through OSFS can be handed to anything that
// takes an OS file or a path. OSFS gains a method when package os gains a
// function.
type OSFS struct{}

func (OSFS) fs() {}

// Open is os.Open.
func (OSFS) Open(name string) (*os.File, error) { return os.Open(name) }

// OpenFile is os.OpenFile.
func (OSFS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

// Create is os.Create.
func (OSFS) Create(name string) (*os.File, error) { return os.Create(name) }

// CreateTemp is os.CreateTemp.
func (OSFS) CreateTemp(dir, pattern string) (*os.File, error) { return os.CreateTemp(dir, pattern) }

// OpenRoot is os.OpenRoot.
func (OSFS) OpenRoot(name string) (*os.Root, error) { return os.OpenRoot(name) }

// OpenInRoot is os.OpenInRoot.
func (OSFS) OpenInRoot(dir, name string) (*os.File, error) { return os.OpenInRoot(dir, name) }

// ReadFile is os.ReadFile.
func (OSFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }

// WriteFile is os.WriteFile.
func (OSFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

// Truncate is os.Truncate.
func (OSFS) Truncate(name string, size int64) error { return os.Truncate(name, size) }

// CopyFS is os.CopyFS.
func (OSFS) CopyFS(dir string, fsys fs.FS) error { return os.CopyFS(dir, fsys) }

// Mkdir is os.Mkdir.
func (OSFS) Mkdir(name string, perm os.FileMode) error { return os.Mkdir(name, perm) }

// MkdirAll is os.MkdirAll.
func (OSFS) MkdirAll(name string, perm os.FileMode) error { return os.MkdirAll(name, perm) }

// MkdirTemp is os.MkdirTemp.
func (OSFS) MkdirTemp(dir, pattern string) (string, error) { return os.MkdirTemp(dir, pattern) }

// ReadDir is os.ReadDir.
func (OSFS) ReadDir(name string) ([]os.DirEntry, error) { return os.ReadDir(name) }

// Remove is os.Remove.
func (OSFS) Remove(name string) error { return os.Remove(name) }

// RemoveAll is os.RemoveAll.
func (OSFS) RemoveAll(name string) error { return os.RemoveAll(name) }

// Rename is os.Rename.
func (OSFS) Rename(oldname, newname string) error { return os.Rename(oldname, newname) }

// Link is os.Link.
func (OSFS) Link(oldname, newname string) error { return os.Link(oldname, newname) }

// Symlink is os.Symlink.
func (OSFS) Symlink(oldname, newname string) error { return os.Symlink(oldname, newname) }

// Readlink is os.Readlink.
func (OSFS) Readlink(name string) (string, error) { return os.Readlink(name) }

// Stat is os.Stat.
func (OSFS) Stat(name string) (os.FileInfo, error) { return os.Stat(name) }

// Lstat is os.Lstat.
func (OSFS) Lstat(name string) (os.FileInfo, error) { return os.Lstat(name) }

// Chmod is os.Chmod.
func (OSFS) Chmod(name string, mode os.FileMode) error { return os.Chmod(name, mode) }

// Chown is os.Chown.
func (OSFS) Chown(name string, uid, gid int) error { return os.Chown(name, uid, gid) }

// Lchown is os.Lchown.
func (OSFS) Lchown(name string, uid, gid int) error { return os.Lchown(name, uid, gid) }

// Chtimes is os.Chtimes.
func (OSFS) Chtimes(name string, atime, mtime time.Time) error {
	return os.Chtimes(name, atime, mtime)
}

// Getwd returns the absolute path that relative names are relative to.
func (OSFS) Getwd() (string, error) { return os.Getwd() }

// DirFS returns an fs.FS view of dir, for fs.WalkDir, fs.Glob,
// template.ParseFS and the other std helpers.
func (OSFS) DirFS(dir string) fs.FS { return os.DirFS(dir) }
