// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package host_test

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host"
)

// Every test in this module works inside a directory from t.TempDir and
// opens no socket. hosttest's conformance table runs OS over real loopback
// sockets.

func TestOS_has_every_field(t *testing.T) {
	h := host.OS()

	require.NotNil(t, h.Listen)
	require.NotNil(t, h.Dial)
	require.Equal(t, host.OSFS{}, h.FS)
}

func TestOS_Listen_is_net_Listen(t *testing.T) {
	h := host.OS()

	got := reflect.ValueOf(h.Listen).Pointer()
	want := reflect.ValueOf(net.Listen).Pointer()
	require.Equal(t, want, got, "Listen is not net.Listen")
}

// frameworkFS is the filesystem a framework built on bedrock might expose:
// an interface that embeds host.FS and lists the methods it needs.
type frameworkFS interface {
	host.FS

	Open(name string) (*os.File, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	Stat(name string) (os.FileInfo, error)
}

// frameworkHost is how such a framework hides the type parameter.
type frameworkHost = host.Host[frameworkFS]

func TestOSFS_is_a_framework_filesystem(t *testing.T) {
	o := host.OS()
	h := frameworkHost{
		Listen: o.Listen,
		Dial:   o.Dial,
		FS:     o.FS,
	}

	name := filepath.Join(t.TempDir(), "a.txt")
	require.NoError(t, h.FS.WriteFile(name, []byte("framework"), 0o600))

	b, err := h.FS.ReadFile(name)
	require.NoError(t, err)
	require.Equal(t, "framework", string(b))

	info, err := h.FS.Stat(name)
	require.NoError(t, err)
	require.Equal(t, int64(len("framework")), info.Size())

	f, err := h.FS.Open(name)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
