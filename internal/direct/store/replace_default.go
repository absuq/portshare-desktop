//go:build !windows

package store

import "os"

func replaceExistingFile(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
