package staticcheckfuncusedonlybytests

// Copyright (c) 2025, João Breno. See the license.

import "testing"

func TestSum(t *testing.T) {
	// "staticcheck -test=false -checks=U1000" will report that sum is not used, even though it is used by this test.
	if s := sum(10, 20, 30); s != 60 {
		t.Errorf("sum(10, 20, 30) == %v, want 60", s)
	}
}
