package testok

import "testing"

func TestSum(t *testing.T) {
	if s := sum(10, 20, 30); s != 60 {
		t.Errorf("sum(10, 20, 30) == %v, want 60", s)
	}
}
