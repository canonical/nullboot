// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package main

import "github.com/canonical/nullboot/efibootmgr"
import "flag"
import "log"
import "os"

var noTPM = flag.Bool("no-tpm", false, "Do not do any resealing with the TPM")
var noEfivars = flag.Bool("no-efivars", false, "Do not use or update the EFI variables")
var outputJSON = flag.String("output-json", "", "JSON file to write (also disables writing real EFI variables)")

func main() {
	var assets *efibootmgr.TrustedAssets
	var err error
	flag.Parse()

	const (
		esp             = "/boot/efi"
		shimSourceDir   = "/usr/lib/nullboot/shim"
		kernelSourceDir = "/usr/lib/linux/efi"
		vendor          = "ubuntu"
	)

	// FIXME: Let's actually add some arg parsing and stuff?
	if !*noTPM {
		assets, err = efibootmgr.ReadTrustedAssets()
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

		if err := efibootmgr.TrustCurrentBoot(assets, esp); err != nil {
			log.Println("cannot trust boot assets used for current boot:", err)
			os.Exit(1)
		}
	}

	var maybeBm *efibootmgr.BootManager
	var efivars efibootmgr.EFIVariables
	if *outputJSON != "" {
		efivars = &efibootmgr.MockEFIVariables{}
	} else {
		efivars = efibootmgr.RealEFIVariables{}
	}
	if !*noEfivars {
		if bm, err := efibootmgr.NewBootManagerForVariables(efivars); err != nil {
			log.Println("cannot load efi boot variables:", err)
			os.Exit(1)
		} else {
			maybeBm = &bm
		}
	}

	km, err := efibootmgr.NewKernelManager(esp, kernelSourceDir, vendor, maybeBm)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	if assets != nil {
		if err := assets.Save(); err != nil {
			log.Println("cannot update list of trusted boot assets:", err)
			os.Exit(1)
		}

		// Initial reseal against new assets
		if err := efibootmgr.ResealKey(assets, km, esp, shimSourceDir, vendor); err != nil {
			log.Println("initial reseal failed:", err)
			os.Exit(1)
		}
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

	if assets != nil {
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

	if jsonEfivars := efivars.(*efibootmgr.MockEFIVariables); jsonEfivars != nil {
		json, err := jsonEfivars.JSON()
		if err != nil {
			log.Println("cannot write json:", err)
			os.Exit(2)
		}

		f, err := os.Create(*outputJSON)
		if err != nil {
			log.Printf("Could not open JSON output file %s: %v", *outputJSON, err)
			os.Exit(1)
		}
		defer f.Close()

		_, err = f.Write(json)
		if err != nil {
			log.Printf("Could not write JSON output file %s: %v", *outputJSON, err)
			os.Exit(1)
		}
	}
}
