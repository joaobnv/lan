package opfa

import (
	"ps/exptf"
)

// Copyright (c) 2025, João Breno. See the license.

func FuzzPsfOpFa(f *exptf.F) {
	f.Add(1, 2)
	f.Add(2, 3)
	f.Fuzz(func(t *exptf.T, a, b int) {
		if s := Sub(a, b); s != a-b {
			t.Errorf("Sub(%d, %d) = %d, want %d", a, b, s, a-b)
		}
	})
}
