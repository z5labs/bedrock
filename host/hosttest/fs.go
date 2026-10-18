// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/z5labs/bedrock/host"
)

// tempDirName is the directory inside a Host's directory that CreateTemp
// and MkdirTemp use when dir is "", as os.TempDir is for package os.
const tempDirName = "tmp"

// WithFiles writes files into the Host's directory, keyed by their path
// relative to it with forward slashes, such as "etc/config.yaml". Parent
// directories are created.
//
// Paths use forward slashes on every OS. A path that is absolute, or that
// leaves the directory, fails the test through the testing.TB given to
// New, naming the path. The map and its contents are copied when WithFiles
// is called, so changing them afterwards changes nothing.
func WithFiles(files map[string][]byte) Option {
	files = maps.Clone(files)
	for name, data := range files {
		files[name] = bytes.Clone(data)
	}
	return func(h *Host) {
		h.tb.Helper()
		for _, name := range slices.Sorted(maps.Keys(files)) {
			if err := writeFile(h.root, name, files[name]); err != nil {
				h.tb.Errorf("hosttest: WithFiles: %q: %v", name, err)
			}
		}
	}
}

func writeFile(root *os.Root, name string, data []byte) error {
	rel, err := local(name)
	if err != nil {
		return err
	}
	if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		return err
	}
	return root.WriteFile(rel, data, 0o644)
}

// errNotLocal is the cause of every error for a name outside a Host's
// directory.
var errNotLocal = errors.New("not a path inside the Host's directory")

// local converts name, written with forward slashes and relative to a
// Host's directory, to a relative OS path, or fails if it is absolute or
// leaves the directory.
func local(name string) (string, error) {
	p := filepath.FromSlash(name)
	if filepath.IsAbs(p) || !filepath.IsLocal(p) {
		return "", errNotLocal
	}
	return p, nil
}

// FS returns the Host's filesystem.
func (h *Host) FS() *FS {
	return &FS{dir: h.dir, root: h.root}
}

// Path returns the absolute path of name, relative to the Host's
// directory, to put in an argument or a setting.
//
// Like WithFiles, Path takes forward slashes on every OS, and fails the
// test through the testing.TB given to New, naming the path, if name is
// absolute or leaves the directory.
func (h *Host) Path(name string) string {
	rel, err := local(name)
	if err != nil {
		h.tb.Helper()
		h.tb.Errorf("hosttest: Path(%q): %v", name, err)
		return filepath.Join(h.dir, filepath.FromSlash(name))
	}
	return filepath.Join(h.dir, rel)
}

// sealed is host.FS under a name that is not exported, so that embedding
// it gives FS host.FS's method without a field in FS's docs.
type sealed = host.FS //nolint:unused // used only as FS's embedded field

// FS is a filesystem confined to a Host's directory with os.Root. It has
// every method of host.OSFS, with the same results, except that a name
// that leaves the directory fails, and that Symlink and CopyFS refuse a
// symbolic link to an absolute target, which os.Root would never follow.
// Its Getwd is the Host's directory.
//
// It is confined with os.Root, so ".." and symbolic links can't leave the
// directory. FS accepts absolute names under the directory, so that a
// file's Name() can be passed back in, as with host.OSFS. Relative names
// are relative to the Host's directory. Files are opened under its
// absolute path, so a file's Name() is an absolute OS path, and MkdirTemp
// returns one. CreateTemp and MkdirTemp with dir == "" use "tmp" inside
// the directory, created when it is first needed. Every error names the
// path the caller passed.
//
// FS satisfies host.FS, so it is the filesystem of a host.Host whose
// filesystem is an interface that embeds host.FS and lists methods of FS.
type FS struct {
	// sealed is never set and its method is never called: it is embedded
	// only for the method set. FS must not embed host.OSFS for it, since
	// every method FS lacked would then be the real filesystem's, and
	// would compile, and would escape the directory.
	sealed //nolint:unused // embedded for its method set, never set or called

	dir  string
	root *os.Root
}

// rel returns the name to give the os.Root for name: name itself when it
// is relative, and its path relative to the directory when it is
// absolute. A name outside the directory comes back starting with "..",
// and the os.Root refuses it.
func (f *FS) rel(name string) string {
	if !filepath.IsAbs(name) {
		return name
	}
	rel, err := filepath.Rel(f.dir, name)
	if err != nil {
		// Only on Windows, for a name on another volume. Any path that
		// escapes serves: the os.Root refuses it.
		return ".." + string(filepath.Separator) + name
	}
	return rel
}

// abs returns the absolute OS path of rel, a name relative to the
// directory.
func (f *FS) abs(rel string) string {
	return filepath.Join(f.dir, rel)
}

// renamed puts name back in a *fs.PathError the os.Root returned for rel,
// so that an error names the path the caller passed.
func renamed(err error, rel, name string) error {
	var pe *fs.PathError
	if errors.As(err, &pe) && pe.Path == rel {
		pe.Path = name
	}
	return err
}

// Open is host.OSFS's Open, confined to the Host's directory.
func (f *FS) Open(name string) (*os.File, error) {
	rel := f.rel(name)
	file, err := f.root.Open(rel)
	return file, renamed(err, rel, name)
}

// OpenFile is host.OSFS's OpenFile, confined to the Host's directory.
func (f *FS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	rel := f.rel(name)
	file, err := f.root.OpenFile(rel, flag, perm)
	return file, renamed(err, rel, name)
}

// Create is host.OSFS's Create, confined to the Host's directory.
func (f *FS) Create(name string) (*os.File, error) {
	rel := f.rel(name)
	file, err := f.root.Create(rel)
	return file, renamed(err, rel, name)
}

// OpenRoot is host.OSFS's OpenRoot, confined to the Host's directory. The
// *os.Root it returns is confined to the directory name.
func (f *FS) OpenRoot(name string) (*os.Root, error) {
	rel := f.rel(name)
	root, err := f.root.OpenRoot(rel)
	return root, renamed(err, rel, name)
}

// OpenInRoot is host.OSFS's OpenInRoot, confined to the Host's directory:
// dir may not leave the Host's directory, and name may not leave dir.
func (f *FS) OpenInRoot(dir, name string) (*os.File, error) {
	root, err := f.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	file, err := root.Open(name)
	// The file holds a descriptor of its own, so closing the root can't
	// change it, and os.OpenInRoot does not report the close either.
	_ = root.Close()
	return file, err
}

// ReadFile is host.OSFS's ReadFile, confined to the Host's directory.
func (f *FS) ReadFile(name string) ([]byte, error) {
	rel := f.rel(name)
	data, err := f.root.ReadFile(rel)
	// Reading a directory fails naming the file's Name(), which is
	// absolute, so it is put back to the name passed.
	return data, renamed(renamed(err, rel, name), joinPath(f.dir, rel), name)
}

// WriteFile is host.OSFS's WriteFile, confined to the Host's directory.
func (f *FS) WriteFile(name string, data []byte, perm os.FileMode) error {
	rel := f.rel(name)
	err := f.root.WriteFile(rel, data, perm)
	return renamed(renamed(err, rel, name), joinPath(f.dir, rel), name)
}

// Truncate is host.OSFS's Truncate, confined to the Host's directory. It
// opens the file to truncate it, so it fails, as os.Truncate does, for a
// directory, and every error is a *fs.PathError naming the name passed.
func (f *FS) Truncate(name string, size int64) error {
	file, err := f.OpenFile(name, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	err = renamed(file.Truncate(size), file.Name(), name)
	if cerr := file.Close(); err == nil {
		err = renamed(cerr, file.Name(), name)
	}
	return err
}

// CopyFS is host.OSFS's CopyFS, confined to the Host's directory. Like
// os.CopyFS it never overwrites a file, failing with fs.ErrExist. It
// copies a symbolic link with Symlink, so an fsys holding a symbolic link
// to an absolute target fails as Symlink does.
func (f *FS) CopyFS(dir string, fsys fs.FS) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fpath, err := filepath.Localize(path)
		if err != nil {
			return err
		}
		target := joinPath(dir, fpath)

		switch d.Type() {
		case fs.ModeDir:
			return f.MkdirAll(target, 0o777)
		case fs.ModeSymlink:
			link, err := fs.ReadLink(fsys, path)
			if err != nil {
				return err
			}
			return f.Symlink(link, target)
		case 0:
			return f.copyFile(fsys, path, target)
		default:
			return &fs.PathError{Op: "CopyFS", Path: path, Err: fs.ErrInvalid}
		}
	})
}

// copyFile copies the regular file path of fsys to target, as os.CopyFS
// does.
func (f *FS) copyFile(fsys fs.FS, path, target string) error {
	r, err := fsys.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }() // read only: a failed close loses nothing
	info, err := r.Stat()
	if err != nil {
		return err
	}
	w, err := f.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666|info.Mode()&0o777)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close() // the copy has already failed, and is what is reported
		return &fs.PathError{Op: "Copy", Path: target, Err: err}
	}
	return renamed(w.Close(), w.Name(), target)
}

// Link is host.OSFS's Link, confined to the Host's directory: neither name
// may leave it.
func (f *FS) Link(oldname, newname string) error {
	oldrel, newrel := f.rel(oldname), f.rel(newname)
	return relinked(f.root.Link(oldrel, newrel), oldrel, oldname, newrel, newname)
}

// Symlink is host.OSFS's Symlink, confined to the Host's directory, except
// that it refuses an absolute oldname, creating nothing, with an
// *os.LinkError whose Err is errors.ErrUnsupported. The directory is
// confined with os.Root, which won't follow an absolute symbolic link even
// to a file inside the directory, so such a link would fail at its first
// use rather than here.
//
// A relative oldname is stored as written. A link that leaves the
// directory is created, but following it fails.
func (f *FS) Symlink(oldname, newname string) error {
	if filepath.IsAbs(oldname) {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: errors.ErrUnsupported}
	}
	newrel := f.rel(newname)
	return relinked(f.root.Symlink(oldname, newrel), oldname, oldname, newrel, newname)
}

// Readlink is host.OSFS's Readlink, confined to the Host's directory. It
// returns the target as it was written.
func (f *FS) Readlink(name string) (string, error) {
	rel := f.rel(name)
	target, err := f.root.Readlink(rel)
	return target, renamed(err, rel, name)
}

// Lstat is host.OSFS's Lstat, confined to the Host's directory.
func (f *FS) Lstat(name string) (os.FileInfo, error) {
	rel := f.rel(name)
	fi, err := f.root.Lstat(rel)
	return fi, renamed(err, rel, name)
}

// Chmod is host.OSFS's Chmod, confined to the Host's directory.
func (f *FS) Chmod(name string, mode os.FileMode) error {
	rel := f.rel(name)
	return renamed(f.root.Chmod(rel, mode), rel, name)
}

// Chown is host.OSFS's Chown, confined to the Host's directory.
func (f *FS) Chown(name string, uid, gid int) error {
	rel := f.rel(name)
	return renamed(f.root.Chown(rel, uid, gid), rel, name)
}

// Lchown is host.OSFS's Lchown, confined to the Host's directory.
func (f *FS) Lchown(name string, uid, gid int) error {
	rel := f.rel(name)
	return renamed(f.root.Lchown(rel, uid, gid), rel, name)
}

// Chtimes is host.OSFS's Chtimes, confined to the Host's directory.
func (f *FS) Chtimes(name string, atime, mtime time.Time) error {
	rel := f.rel(name)
	return renamed(f.root.Chtimes(rel, atime, mtime), rel, name)
}

// Getwd returns the Host's directory, an absolute path, which relative
// names are relative to. It never fails.
func (f *FS) Getwd() (string, error) {
	return f.dir, nil
}

// tempDir returns the name, relative to the directory, of the directory
// that CreateTemp or MkdirTemp creates in, and the name to report it by
// in errors: dir both times, or for "" the Host's temporary directory,
// which it creates, and its absolute path.
func (f *FS) tempDir(dir string) (rel, shown string, err error) {
	if dir != "" {
		return f.rel(dir), dir, nil
	}
	if err := f.root.MkdirAll(tempDirName, 0o755); err != nil {
		return "", "", renamed(err, tempDirName, f.abs(tempDirName))
	}
	return tempDirName, f.abs(tempDirName), nil
}

// tempPattern splits pattern at its last "*", as os.CreateTemp does.
func tempPattern(op, pattern string) (prefix, suffix string, err error) {
	if strings.ContainsRune(pattern, os.PathSeparator) || strings.ContainsRune(pattern, '/') {
		return "", "", &fs.PathError{Op: op, Path: pattern, Err: errors.New("pattern contains path separator")}
	}
	prefix = pattern
	if i := strings.LastIndexByte(pattern, '*'); i >= 0 {
		prefix, suffix = pattern[:i], pattern[i+1:]
	}
	return prefix, suffix, nil
}

// random returns the random part of a name CreateTemp or MkdirTemp tries.
func random() string {
	return strconv.FormatUint(uint64(rand.Uint32()), 10)
}

// joinPath joins dir and name as package os does for CreateTemp and
// MkdirTemp, so that their errors name dir as the caller wrote it.
func joinPath(dir, name string) string {
	if len(dir) > 0 && os.IsPathSeparator(dir[len(dir)-1]) {
		return dir + name
	}
	return dir + string(os.PathSeparator) + name
}

// tempTries is how many names CreateTemp and MkdirTemp try before giving
// up, as package os does.
const tempTries = 10000

// CreateTemp is host.OSFS's CreateTemp, confined to the Host's directory.
// With dir == "" it creates the file in "tmp" inside the directory.
func (f *FS) CreateTemp(dir, pattern string) (*os.File, error) {
	prefix, suffix, err := tempPattern("createtemp", pattern)
	if err != nil {
		return nil, err
	}
	return f.createTemp(dir, prefix, suffix, random)
}

// createTemp is CreateTemp, trying names whose random part random makes.
func (f *FS) createTemp(dir, prefix, suffix string, random func() string) (*os.File, error) {
	d, shown, err := f.tempDir(dir)
	if err != nil {
		return nil, err
	}
	for range tempTries {
		base := prefix + random() + suffix
		rel := filepath.Join(d, base)
		file, err := f.root.OpenFile(rel, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return file, renamed(err, rel, joinPath(shown, base))
	}
	return nil, &fs.PathError{Op: "createtemp", Path: joinPath(shown, prefix) + "*" + suffix, Err: fs.ErrExist}
}

// MkdirTemp is host.OSFS's MkdirTemp, confined to the Host's directory.
// With dir == "" it creates the directory in "tmp" inside the Host's
// directory. It returns an absolute OS path.
func (f *FS) MkdirTemp(dir, pattern string) (string, error) {
	prefix, suffix, err := tempPattern("mkdirtemp", pattern)
	if err != nil {
		return "", err
	}
	return f.mkdirTemp(dir, prefix, suffix, random)
}

// mkdirTemp is MkdirTemp, trying names whose random part random makes.
func (f *FS) mkdirTemp(dir, prefix, suffix string, random func() string) (string, error) {
	d, shown, err := f.tempDir(dir)
	if err != nil {
		return "", err
	}
	for range tempTries {
		base := prefix + random() + suffix
		rel := filepath.Join(d, base)
		err := f.root.Mkdir(rel, 0o700)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if errors.Is(err, fs.ErrNotExist) {
			// As os.MkdirTemp does, a missing dir is reported as the
			// failure to stat dir, not to make the name inside it.
			if _, serr := f.root.Stat(d); errors.Is(serr, fs.ErrNotExist) {
				return "", renamed(serr, d, shown)
			}
		}
		if err != nil {
			return "", renamed(err, rel, joinPath(shown, base))
		}
		return f.abs(rel), nil
	}
	return "", &fs.PathError{Op: "mkdirtemp", Path: joinPath(shown, prefix) + "*" + suffix, Err: fs.ErrExist}
}

// Mkdir is host.OSFS's Mkdir, confined to the Host's directory.
func (f *FS) Mkdir(name string, perm os.FileMode) error {
	rel := f.rel(name)
	return renamed(f.root.Mkdir(rel, perm), rel, name)
}

// MkdirAll is host.OSFS's MkdirAll, confined to the Host's directory.
func (f *FS) MkdirAll(name string, perm os.FileMode) error {
	rel := f.rel(name)
	err := f.root.MkdirAll(rel, perm)
	if errors.Is(err, fs.ErrExist) {
		// os.Root reports a name that is not a directory as existing.
		// os.MkdirAll reports it as not a directory, unless it is a
		// symbolic link that can't be followed.
		if _, serr := f.root.Stat(rel); serr == nil {
			return &fs.PathError{Op: "mkdir", Path: name, Err: syscall.ENOTDIR}
		}
	}
	return renamed(err, rel, name)
}

// Remove is host.OSFS's Remove, confined to the Host's directory.
func (f *FS) Remove(name string) error {
	rel := f.rel(name)
	return renamed(f.root.Remove(rel), rel, name)
}

// RemoveAll is host.OSFS's RemoveAll, confined to the Host's directory. A
// symbolic link is removed, not followed.
func (f *FS) RemoveAll(name string) error {
	rel := f.rel(name)
	return renamed(f.root.RemoveAll(rel), rel, name)
}

// Rename is host.OSFS's Rename, confined to the Host's directory: neither
// name may leave it.
func (f *FS) Rename(oldname, newname string) error {
	oldrel, newrel := f.rel(oldname), f.rel(newname)
	return relinked(f.root.Rename(oldrel, newrel), oldrel, oldname, newrel, newname)
}

// relinked puts oldname and newname back in an *os.LinkError the os.Root
// returned for oldrel and newrel, and oldname in a *fs.PathError, so that
// an error names the paths the caller passed.
func relinked(err error, oldrel, oldname, newrel, newname string) error {
	var le *os.LinkError
	if errors.As(err, &le) {
		if le.Old == oldrel {
			le.Old = oldname
		}
		if le.New == newrel {
			le.New = newname
		}
	}
	return renamed(err, oldrel, oldname)
}

// Stat is host.OSFS's Stat, confined to the Host's directory.
func (f *FS) Stat(name string) (os.FileInfo, error) {
	rel := f.rel(name)
	fi, err := f.root.Stat(rel)
	return fi, renamed(err, rel, name)
}

// ReadDir is host.OSFS's ReadDir, confined to the Host's directory. Its
// entries are sorted by filename.
func (f *FS) ReadDir(name string) ([]os.DirEntry, error) {
	rel := f.rel(name)
	file, err := f.root.Open(rel)
	if err != nil {
		return nil, renamed(err, rel, name)
	}
	entries, err := file.ReadDir(-1)
	// Reading a file that is not a directory fails naming the file's
	// Name(), which is absolute, so it is put back to the name passed.
	err = renamed(err, file.Name(), name)
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return entries, err
}

// DirFS is host.OSFS's DirFS, confined to the Host's directory. For a dir
// outside it, every Open of the fs.FS fails, since DirFS cannot.
func (f *FS) DirFS(dir string) fs.FS {
	name := f.rel(dir)
	if name != "" {
		// os.DirFS takes "./dir" and "dir/", which fs.Sub does not, and
		// fails for "", which Clean would make the whole directory.
		name = filepath.Clean(name)
	}
	sub, err := fs.Sub(f.root.FS(), filepath.ToSlash(name))
	if err != nil {
		return errFS{err: &fs.PathError{Op: "dirfs", Path: dir, Err: errNotLocal}}
	}
	return sub
}

// errFS is the fs.FS DirFS returns for a dir outside the Host's
// directory: like os.DirFS, DirFS can't fail, so every Open does.
type errFS struct{ err error }

func (e errFS) Open(string) (fs.File, error) {
	return nil, e.err
}
