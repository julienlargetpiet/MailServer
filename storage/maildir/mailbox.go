package maildir

import (
	"os"
	"path/filepath"
)

func ensureMaildir(path string) error {
	dirs := []string{
		filepath.Join(path, "cur"),
		filepath.Join(path, "new"),
		filepath.Join(path, "tmp"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}

	return nil
}
