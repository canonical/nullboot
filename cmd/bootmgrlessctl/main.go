// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package main

import "github.com/canonical/bootmgrless/efibootmgr"
import "log"
import "os"

func main() {
	// FIXME: Let's actually add some arg parsing and stuff?
	km, err := efibootmgr.NewKernelManager()
	if err != nil {
		log.Print(err)
		os.Exit(1)
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
