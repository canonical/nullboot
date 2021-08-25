// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efivars

import (
	"os"
	"testing"
)

func TestNewDevicePath_smoke(t *testing.T) {
	const bootable = "/boot/efi/EFI/BOOT/BOOTX64.EFI"

	if _, err := os.Stat(bootable); err != nil {
		t.Skip("Smoke test file not found")
	}

	out, err := NewDevicePath(bootable, 0)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(out) < len(bootable)*2 {
		t.Fatalf("output is too short")
	}

	out2, err := NewDevicePath(bootable, BootAbbrevHD)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(out2) < len(bootable)*2 || len(out2) > len(out) {
		t.Fatalf("output is too short")
	}
}
