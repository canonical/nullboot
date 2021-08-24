// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"testing"
)

func TestBootManagerNextFreeKernel(t *testing.T) {
	bm := BootManager{}
	bm.entries = map[int]BootEntryVariable{
		0: {},
		2: {},
		5: {},
	}

	wants := []int{1, 3, 4, 6, 7}

	for i, want := range wants {
		got, err := bm.AddEntry("desc", "path", []string{"arg1", "arg2"})
		if err != nil {
			t.Fatalf("expected: %v, got error: %v at step %d", want, err, i)
		} else if got != want {
			t.Fatalf("expected: %v, got: %v at step %d", want, got, i)
		}
	}
}
