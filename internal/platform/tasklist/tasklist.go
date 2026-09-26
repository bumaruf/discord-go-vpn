// Package tasklist finds running processes on Windows by image name.
package tasklist

import (
	"encoding/csv"
	"io"
	"os/exec"
	"slices"
	"strings"
)

// Running reports whether a process with one of names runs. A single
// /FI IMAGENAME filter matches only one name, so it lists every process.
func Running(names ...string) bool {
	out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	return has(string(out), names)
}

// has reports whether the output of "tasklist /FO CSV /NH" lists a process
// with one of names.
func has(output string, names []string) bool {
	r := csv.NewReader(strings.NewReader(output))
	r.FieldsPerRecord = -1
	for {
		record, err := r.Read()
		if err == io.EOF {
			return false
		}
		if err != nil || len(record) == 0 {
			continue
		}
		// Windows image names are case-insensitive, as taskkill /IM matches them.
		if slices.ContainsFunc(names, func(name string) bool {
			return strings.EqualFold(record[0], name)
		}) {
			return true
		}
	}
}
