// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package main

import "github.com/canonical/nullboot/efibootmgr"
import "flag"
import "log"
import "os"

func main() {
	flag.Parse()

	// FIXME: Let's actually add some arg parsing and stuff?
	km, err := efibootmgr.NewKernelManager("/run/mnt/ubuntu-seed", "/usr/lib/linux/efi", "ubuntu")
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	// Install the shim
	updatedShim, err := efibootmgr.InstallShim("/run/mnt/ubuntu-seed", "/usr/lib/nullboot/shim", "ubuntu")
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	if updatedShim {
		log.Print("Updated shim")
	}
	// Install new kernels and commit to bootloader config. This
	// way
	if err = km.InstallKernels(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	if err = km.CommitToBootLoader(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	// Cleanup old entries
	if err = km.RemoveObsoleteKernels(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	if err = km.CommitToBootLoader(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
