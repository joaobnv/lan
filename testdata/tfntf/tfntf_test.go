package tfntf

// Copyright (c) 2025, João Breno. See the license.

import (
	"testing"
)

func TNP(t *testing.T) {
	t.Fail()
}

func Testlc(t *testing.T) {
	t.Fail()
}

func FNP(f *testing.F) {
	f.Fail()
}

func Fuzzlc(f *testing.F) {
	a := []string{"not a number"}
	a = append(a)
	f.Errorf("%d", a)
}
