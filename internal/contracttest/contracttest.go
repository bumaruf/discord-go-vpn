// Package contracttest checks Go types against the wire fixtures in testdata/contracts,
// which the panel's tests check against its TypeScript types.
package contracttest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Read returns the contents of testdata/contracts/<name>.
func Read(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata", "contracts", name))
	if err != nil {
		t.Fatalf("reading contract fixture: %v", err)
	}
	return data
}

// AssertRoundTrip decodes the fixture into T, rejecting keys T doesn't know, and
// checks that encoding the value gives back the same JSON. A key renamed in the
// fixture or in a json tag fails either way.
func AssertRoundTrip[T any](t testing.TB, name string) {
	t.Helper()
	fixture := Read(t, name)

	decoder := json.NewDecoder(bytes.NewReader(fixture))
	decoder.DisallowUnknownFields()
	var value T
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decoding %s into %T: %v", name, value, err)
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding %T: %v", value, err)
	}

	var want, got any
	if err := json.Unmarshal(fixture, &want); err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("parsing encoded %T: %v", value, err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("%s does not round-trip through %T\nfixture: %s\nencoded: %s", name, value, compact(fixture), encoded)
	}
}

// repoRoot walks up from the test's working directory, which is its package directory.
func repoRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the working directory")
		}
		dir = parent
	}
}

func compact(data []byte) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return data
	}
	return buf.Bytes()
}
