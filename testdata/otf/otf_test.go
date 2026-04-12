package otf

// Copyright (c) 2026, João Breno. See the license.

import . "testing"

func FuzzPsfid(f *F) {
	f.Add(2, 1)
	f.Add(3, 2)
	f.Fuzz(func(t *T, a, b int) {
		if a-b != 1 {
			t.Errorf("%d - %d != 1", a, b)
		}
	})
}
