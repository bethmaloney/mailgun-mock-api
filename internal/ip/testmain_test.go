package ip_test

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("IP_TEST_SUBPROCESS") == "1" {
		os.Exit(m.Run())
	}
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

func runTestInSubprocess(testName string) bool {
	cmd := exec.Command(os.Args[0], "-test.run", "^"+testName+"$", "-test.v")
	cmd.Env = append(os.Environ(), "IP_TEST_SUBPROCESS=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "--- FAIL: %s\n", testName)
		return false
	}
	return true
}
