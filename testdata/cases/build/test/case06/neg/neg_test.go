package neg

// Copyright (c) 2026, João Breno. See the license.

import "testing"

func TestNeg(t *struct{ t *testing.T }) {
	if Neg(1+2i) != -1-2i {
		t.t.Fail()
	}
}
