package main

import (
	"testing"
)

func TestHelpWithoutSystem(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"version"}, {"help", "run"}, {"apply", "-h"}} {
		if code := execute(args); code != 0 {
			t.Fatal(args, code)
		}
	}
}
func TestInvalidCLI(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"run", "-n"}, {"apply", "-c"}, {"apply", "-c", "a", "-c", "b"}} {
		if code := execute(args); code != 2 {
			t.Fatal(args, code)
		}
	}
}
