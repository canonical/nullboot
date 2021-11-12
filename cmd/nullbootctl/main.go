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

	const (
		esp             = "/run/mnt/ubuntu-seed"
		shimSourceDir   = "/usr/lib/nullboot/shim"
		kernelSourceDir = "/usr/lib/linux/efi"
		vendor          = "ubuntu"
	)

	// FIXME: Let's actually add some arg parsing and stuff?
	assets, err := efibootmgr.ReadTrustedAssets()
	if err != nil {
		log.Println("cannot read trusted asset hashes:", err)
		os.Exit(1)
	}

	for _, p := range []string{shimSourceDir, kernelSourceDir} {
		if err := assets.TrustNewFromDir(p); err != nil {
			log.Println("cannot add new assets from", p, ":", err)
			os.Exit(1)
		}
	}

	km, err := efibootmgr.NewKernelManager(esp, kernelSourceDir, vendor)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	if err := assets.Save(); err != nil {
		log.Println("cannot update list of trusted boot assets:", err)
		os.Exit(1)
	}

	// Initial reseal against new assets
	if err := efibootmgr.ResealKey(assets, km, esp, shimSourceDir, vendor); err != nil {
		log.Println("initial reseal failed:", err)
		os.Exit(1)
	}

	// Install the shim
	updatedShim, err := efibootmgr.InstallShim(esp, shimSourceDir, vendor)
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

	assets.RemoveObsolete()
	if err := assets.Save(); err != nil {
		log.Println("cannot update list of trusted boot assets:", err)
		os.Exit(1)
	}

	// Final reseal to remove obsolete assets from profile
	if err := efibootmgr.ResealKey(assets, km, esp, shimSourceDir, vendor); err != nil {
		log.Println("final reseal failed:", err)
		os.Exit(1)
	}
}
