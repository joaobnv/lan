package pt_test

import (
	"pt"
	"regexp"
	"testing"
)

func TestSum(t *testing.T) {
	regexp.Compile("(") //lint:ignore SA1000 testing
	if s := pt.Sum(1, 2); s != 3 {
		t.Errorf("1 + 2 = %d, want 3", s)
	}
}
