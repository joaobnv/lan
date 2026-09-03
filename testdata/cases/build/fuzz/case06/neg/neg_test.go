package neg

// Copyright (c) 2026, João Breno. See the license.

import "testing"

func FuzzNeg(f *struct{ f *testing.F }) {
	if Neg(1+2i) != -1-2i {
		f.f.Fail()
	}
}
