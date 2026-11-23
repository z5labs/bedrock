// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package host_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/z5labs/bedrock/host"
)

// StateFS is the filesystem a command's state store uses: it declares the
// methods it calls, and nothing else, so a filesystem without them fails
// to compile where it's passed.
type StateFS interface {
	MkdirAll(name string, perm os.FileMode) error
	WriteFile(name string, data []byte, perm os.FileMode) error
	ReadFile(name string) ([]byte, error)
}

// saveToken writes token under dir through fsys and reads it back.
func saveToken(fsys StateFS, dir, token string) (string, error) {
	state := filepath.Join(dir, "state")
	if err := fsys.MkdirAll(state, 0o700); err != nil {
		return "", err
	}
	name := filepath.Join(state, "token")
	if err := fsys.WriteFile(name, []byte(token), 0o600); err != nil {
		return "", err
	}
	b, err := fsys.ReadFile(name)
	return string(b), err
}

// A consumer declares the methods of OSFilesystem it calls as an interface
// of its own, here StateFS, and the zero OSFilesystem, the real
// filesystem, satisfies it. So does any type with those methods: the
// interface embeds nothing a filesystem written from scratch can't have.
func ExampleOSFilesystem() {
	// The temporary directory stands in for the command's state directory,
	// which a real command would read from its configuration.
	dir, err := os.MkdirTemp("", "host-example-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()

	token, err := saveToken(host.OSFilesystem{}, dir, "s3cr3t")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("token:", token)
	// Output:
	// token: s3cr3t
}
