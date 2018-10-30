package cache

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// hashSum takes in a file or directory.
// For files, it hashes based on filename + modified time.
// For directories, it does a hashsum of all files.
func hashSum(path string) (string, error) {
	hash := md5.New()

	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		s, err := os.Stat(path)
		if err != nil {
			return nil
		}
		hashableContent := fmt.Sprintf("%s:%v", path, s.ModTime())
		io.WriteString(hash, hashableContent)
		return nil
	})

	if err != nil {
		return "", nil
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
