// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The names CreateTemp and MkdirTemp try are random, so these tests hand
// createTemp and mkdirTemp random parts of their own, to make them
// collide.

// names returns a function returning each of names in turn, and then the
// last one forever.
func names(names ...string) func() string {
	return func() string {
		name := names[0]
		if len(names) > 1 {
			names = names[1:]
		}
		return name
	}
}

func TestFS_createTemp(t *testing.T) {
	t.Run("tries another name when one is taken", func(t *testing.T) {
		h := New(t, WithFiles(map[string][]byte{"taken": nil}))
		f, err := h.FS().createTemp(h.Path("."), "", "", names("taken", "free"))
		require.NoError(t, err)
		require.NoError(t, f.Close())
		require.Equal(t, h.Path("free"), f.Name())
	})

	t.Run("gives up when every name it tries is taken", func(t *testing.T) {
		// A pattern with no "*" is reported with one at its end, as
		// os.CreateTemp reports it, under the dir passed.
		h := New(t, WithFiles(map[string][]byte{"d/xtaken": nil}))
		_, err := h.FS().createTemp("d", "x", "", names("taken"))
		var pe *fs.PathError
		require.ErrorAs(t, err, &pe)
		require.ErrorIs(t, err, fs.ErrExist)
		require.Equal(t, "createtemp", pe.Op)
		require.Equal(t, filepath.FromSlash("d/x*"), pe.Path)
	})
}

func TestFS_mkdirTemp(t *testing.T) {
	t.Run("tries another name when one is taken", func(t *testing.T) {
		h := New(t, WithFiles(map[string][]byte{"taken": nil}))
		name, err := h.FS().mkdirTemp(h.Path("."), "", "", names("taken", "free"))
		require.NoError(t, err)
		require.Equal(t, h.Path("free"), name)
	})

	t.Run("gives up when every name it tries is taken", func(t *testing.T) {
		h := New(t, WithFiles(map[string][]byte{"d/pre-taken.suf": nil}))
		_, err := h.FS().mkdirTemp("d", "pre-", ".suf", names("taken"))
		var pe *fs.PathError
		require.ErrorAs(t, err, &pe)
		require.ErrorIs(t, err, fs.ErrExist)
		require.Equal(t, "mkdirtemp", pe.Op)
		require.Equal(t, filepath.FromSlash("d/pre-*.suf"), pe.Path)
	})
}

// On Windows filepath.Rel fails for a name on another volume. Everywhere,
// it fails for an absolute name against a relative directory, which no
// Host has, but which takes rel down the same path.
func TestFS_rel_escapes_a_name_it_cannot_make_relative(t *testing.T) {
	f := &FS{dir: "relative"}
	name := filepath.Join(t.TempDir(), "x")
	rel := f.rel(name)
	require.False(t, filepath.IsLocal(rel), rel)
	require.Equal(t, ".."+string(filepath.Separator)+name, rel)
}
