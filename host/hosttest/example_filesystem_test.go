// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/z5labs/bedrock/host/hosttest"
)

// ConfigFS is the filesystem a command's configuration loader uses: it
// declares the method it calls, and nothing else.
type ConfigFS interface {
	ReadFile(name string) ([]byte, error)
}

// loadConfig reads the configuration file name through fsys, or returns
// the default when there is none.
func loadConfig(fsys ConfigFS, name string) (string, error) {
	b, err := fsys.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return "port: 8080", nil
	}
	return string(b), err
}

// A consumer declares the methods it calls as an interface of its own,
// here ConfigFS, and the real code hands it host.OSFilesystem{}. A test
// hands it th.Filesystem() instead, which has the same methods, confined
// to the Host's directory.
func ExampleHost_Filesystem() {
	t := new(exampleTB)
	defer t.runCleanups()

	th := hosttest.New(t, hosttest.WithFiles(map[string][]byte{
		"etc/app.yaml": []byte("port: 9090"),
	}))

	cfg, err := loadConfig(th.Filesystem(), "etc/app.yaml")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(cfg)

	cfg, err = loadConfig(th.Filesystem(), "etc/missing.yaml")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(cfg)
	// Output:
	// port: 9090
	// port: 8080
}
