package profile

import (
	"os"
	"path/filepath"
	"sort"
)

// Discover lists the .conf files beside the executable and in the working
// directory, newest first: a freshly downloaded profile is usually the one
// wanted.
func Discover() []string {
	seen := make(map[string]bool)
	var found []string

	for _, dir := range searchDirs() {
		matches, err := filepath.Glob(filepath.Join(dir, "*.conf"))
		if err != nil {
			continue
		}

		for _, match := range matches {
			// example.conf has placeholders where the keys go.
			if filepath.Base(match) == "example.conf" {
				continue
			}

			abs, err := filepath.Abs(match)
			if err != nil || seen[abs] {
				continue
			}
			if info, err := os.Stat(abs); err != nil || info.IsDir() {
				continue
			}

			seen[abs] = true
			found = append(found, abs)
		}
	}

	sort.Slice(found, func(i, j int) bool {
		a, errA := os.Stat(found[i])
		b, errB := os.Stat(found[j])
		if errA != nil || errB != nil {
			return found[i] < found[j]
		}
		return a.ModTime().After(b.ModTime())
	})

	return found
}

func searchDirs() []string {
	var dirs []string

	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		dirs = append(dirs, filepath.Dir(exe))
	}

	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, cwd)
	}

	return dirs
}
