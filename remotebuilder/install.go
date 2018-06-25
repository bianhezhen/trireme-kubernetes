// +build linux,!darwin,!windows

package remotebuilder

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/aporeto-inc/trireme-kubernetes/static/remoteenforcer"
)

// Install install the remoteenforcer binary.
func Install(path, name string) error {

	dest := filepath.Join(path, name)
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		return nil
	}

	data, err := remoteenforcer.Asset("remotebuilder/cmd/remoteenforcer/remoteenforcer")
	if err != nil {
		return err
	}
	err = os.MkdirAll(path, 0x600)
	if err != nil {
		return err
	}

	if err = syscall.Mount("tmpfs", path, "tmpfs", 0, ""); err != nil {
		return err
	}

	if err = ioutil.WriteFile(dest, data, 0700); err != nil {
		return err
	}

	return nil
}

// Uninstall uninstalls the remoteenforcer binary.
func Uninstall(path string) error {

	remoteBinaryPath := "/var/run/aporeto/tmp"

	if err := syscall.Unmount(path, 0); err != nil {
		return err
	}

	if err := os.RemoveAll(remoteBinaryPath); err != nil {
		return err
	}

	return nil
}
