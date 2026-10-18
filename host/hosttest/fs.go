// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest

import (
	"bytes"
	"cmp"
	"errors"
	"io/fs"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

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
// that leaves the directory fails. Its Getwd is the Host's directory.
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

// tempDir returns the name, relative to the directory, of the directory
// that CreateTemp or MkdirTemp creates in: dir, or for "" the Host's
// temporary directory, which it creates.
func (f *FS) tempDir(dir string) (string, error) {
	if dir != "" {
		return f.rel(dir), nil
	}
	if err := f.root.MkdirAll(tempDirName, 0o755); err != nil {
		return "", renamed(err, tempDirName, f.abs(tempDirName))
	}
	return tempDirName, nil
}

// tempName splits pattern at its last "*", as os.CreateTemp does, and
// returns a function making a new random name from it.
func tempName(op, pattern string) (func() string, error) {
	if strings.ContainsRune(pattern, os.PathSeparator) || strings.ContainsRune(pattern, '/') {
		return nil, &fs.PathError{Op: op, Path: pattern, Err: errors.New("pattern contains path separator")}
	}
	prefix, suffix := pattern, ""
	if i := strings.LastIndexByte(pattern, '*'); i >= 0 {
		prefix, suffix = pattern[:i], pattern[i+1:]
	}
	return func() string {
		return prefix + strconv.FormatUint(uint64(rand.Uint32()), 10) + suffix
	}, nil
}

// tempTries is how many names CreateTemp and MkdirTemp try before giving
// up, as package os does.
const tempTries = 10000

// CreateTemp is host.OSFS's CreateTemp, confined to the Host's directory.
// With dir == "" it creates the file in "tmp" inside the directory.
func (f *FS) CreateTemp(dir, pattern string) (*os.File, error) {
	next, err := tempName("createtemp", pattern)
	if err != nil {
		return nil, err
	}
	return f.createTemp(dir, pattern, next)
}

// createTemp is CreateTemp, trying the names next makes.
func (f *FS) createTemp(dir, pattern string, next func() string) (*os.File, error) {
	d, err := f.tempDir(dir)
	if err != nil {
		return nil, err
	}
	for range tempTries {
		rel := filepath.Join(d, next())
		file, err := f.root.OpenFile(rel, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return file, renamed(err, rel, f.abs(rel))
	}
	return nil, &fs.PathError{Op: "createtemp", Path: f.abs(filepath.Join(d, pattern)), Err: fs.ErrExist}
}

// MkdirTemp is host.OSFS's MkdirTemp, confined to the Host's directory.
// With dir == "" it creates the directory in "tmp" inside the Host's
// directory. It returns an absolute OS path.
func (f *FS) MkdirTemp(dir, pattern string) (string, error) {
	next, err := tempName("mkdirtemp", pattern)
	if err != nil {
		return "", err
	}
	return f.mkdirTemp(dir, pattern, next)
}

// mkdirTemp is MkdirTemp, trying the names next makes.
func (f *FS) mkdirTemp(dir, pattern string, next func() string) (string, error) {
	d, err := f.tempDir(dir)
	if err != nil {
		return "", err
	}
	for range tempTries {
		rel := filepath.Join(d, next())
		err := f.root.Mkdir(rel, 0o700)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if errors.Is(err, fs.ErrNotExist) {
			// As os.MkdirTemp does, a missing dir is reported as the
			// failure to stat dir, not to make the name inside it.
			if _, serr := f.root.Stat(d); errors.Is(serr, fs.ErrNotExist) {
				return "", renamed(serr, d, cmp.Or(dir, f.abs(d)))
			}
		}
		if err != nil {
			return "", renamed(err, rel, f.abs(rel))
		}
		return f.abs(rel), nil
	}
	return "", &fs.PathError{Op: "mkdirtemp", Path: f.abs(filepath.Join(d, pattern)), Err: fs.ErrExist}
}

// Mkdir is host.OSFS's Mkdir, confined to the Host's directory.
func (f *FS) Mkdir(name string, perm os.FileMode) error {
	rel := f.rel(name)
	return renamed(f.root.Mkdir(rel, perm), rel, name)
}

// MkdirAll is host.OSFS's MkdirAll, confined to the Host's directory.
func (f *FS) MkdirAll(name string, perm os.FileMode) error {
	rel := f.rel(name)
	return renamed(f.root.MkdirAll(rel, perm), rel, name)
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
	err := f.root.Rename(oldrel, newrel)
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
	sub, err := fs.Sub(f.root.FS(), filepath.ToSlash(f.rel(dir)))
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
