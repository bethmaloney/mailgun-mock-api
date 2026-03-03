package metrics_test

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

// TestMain ensures each test function runs in its own subprocess so that the
// shared-cache in-memory SQLite database (file::memory:?cache=shared) starts
// fresh for every test. Without this isolation, GORM connection pools keep
// idle connections alive, preventing the in-memory database from being
// destroyed between test functions, which causes cross-test data leakage.
func TestMain(m *testing.M) {
	if os.Getenv("METRICS_TEST_SUBPROCESS") == "1" {
		os.Exit(m.Run())
	}

	// Parent process: discover test names and run each in its own subprocess.
	tests := discoverTests(m)
	failed := false
	for _, name := range tests {
		if !runTestInSubprocess(name) {
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
	os.Exit(0)
}

// discoverTests extracts test function names from testing.M via reflection.
func discoverTests(m *testing.M) []string {
	v := reflect.ValueOf(m).Elem()
	testsField := v.FieldByName("tests")
	if !testsField.IsValid() {
		fmt.Fprintln(os.Stderr, "WARNING: could not discover tests via reflection, running in-process")
		os.Exit(m.Run())
	}

	var names []string
	for i := 0; i < testsField.Len(); i++ {
		nameField := testsField.Index(i).FieldByName("Name")
		if nameField.IsValid() {
			names = append(names, nameField.String())
		}
	}
	return names
}

// runTestInSubprocess runs a single named test in a child process and
// returns true if it passed.
func runTestInSubprocess(testName string) bool {
	cmd := exec.Command(os.Args[0], "-test.run", "^"+testName+"$", "-test.v")
	cmd.Env = append(os.Environ(), "METRICS_TEST_SUBPROCESS=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "--- FAIL: %s\n", testName)
		return false
	}
	return true
}
