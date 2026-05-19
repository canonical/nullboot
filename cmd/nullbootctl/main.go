// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"flag"
	"github.com/canonical/nullboot/efibootmgr"
	"github.com/canonical/nullboot/luks2"
	"log"
	"os"
)

const Usage = `usage:

1. nullbootctl
2. nullbootctl -no-tpm -output-json FILE
3. nullbootctl -no-boot-next
4. nullbootctl recovery-key [OPTIONS]

Commands:

  recovery-key
	  Manage FDE recovery keys (LUKS passphrases).
	  Options:
		--create [--device DEVICE] [--name NAME]
		--delete [--device DEVICE] [--name NAME]
		--list   [--device DEVICE]
`

func usage() {
	log.Print(Usage)
	os.Exit(1)
}

func main() {

	if len(os.Args) > 1 {
		if os.Args[1] == "recovery-key" {
			os.Args = os.Args[1:] // Strip the first item
			cmd_recovery_key()
			return
		}
		if os.Args[1] == "-h" || os.Args[1] == "--help" {
			usage()
		}
	}

	cmd_default()
}

const (
	default_cloudimg_encrypted_device = "/dev/disk/by-label/" + efibootmgr.RootfsLabel
	default_recovery_name             = "recovery-0001"
)

func cmd_recovery_key() {

	var devicePath string
	var recoveryName string
	doCreate := flag.Bool("create", false, "Create and set a recovery key")
	doList := flag.Bool("list", false, "List recovery keys")
	doDelete := flag.Bool("delete", false, "Delete a recovery key")
	flag.StringVar(&devicePath, "device", default_cloudimg_encrypted_device, "Device of the encrypted volume")
	flag.StringVar(&recoveryName, "name", default_recovery_name, "Name of the recovery key")

	flag.Parse()

	if *doCreate && *doList {
		log.Println("Options --create and --list cannot be used together")
		os.Exit(1)
	}
	if *doCreate && *doDelete {
		log.Println("Options --create and --delete cannot be used together")
		os.Exit(1)
	}
	if *doDelete && *doList {
		log.Println("Options --delete and --list cannot be used together")
		os.Exit(1)
	}

	var err error
	if *doList {
		err = luks2.ListRecoveryKeys(devicePath)
	} else if *doDelete {
		err = luks2.DeleteRecoveryKey(devicePath, recoveryName)
	} else if *doCreate {
		err = luks2.CreateRecoveryKey(devicePath, recoveryName)
	} else {
		log.Println("Please select at least one action: --create, --list, --delete")
		os.Exit(1)
	}

	if err != nil {
		os.Exit(1)
	}
}

func cmd_default() {
	var assets *efibootmgr.TrustedAssets
	var err error

	noTPM := flag.Bool("no-tpm", false, "Do not do any resealing with the TPM")
	noEfivars := flag.Bool("no-efivars", false, "Do not use or update the EFI variables. Disables kernel fallback mechanism")
	outputJSON := flag.String("output-json", "", "JSON file to write. Disables writing real EFI variables and enablement of the kernel fallback mechanism")
	noBootNext := flag.Bool("no-boot-next", false, "Disables use of BootNext. This flag must be disabled in order to upgrade to a new kernel version.")

	flag.Parse()

	const (
		esp             = "/boot/efi"
		shimSourceDir   = "/usr/lib/nullboot/shim"
		kernelSourceDir = "/usr/lib/linux/efi"
		vendor          = "ubuntu"
	)

	usingRealEFIVars := *outputJSON == "" && !*noEfivars
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

	if err = km.RegisterNewKernelEFIs(); err != nil {
		log.Print(err)
		os.Exit(1)
	}

	// Determine if the fallback mechanism is required
	isCurrentBootLatest := true
	if usingRealEFIVars {
		// Only set fallback if the latest kernel is not booted
		isCurrentBootLatest, err = km.IsCurrentBootLatest()
		if err != nil {
			log.Printf("Unable to determine if the latest kernel is BootCurrent: %v", err)
			os.Exit(1)
		}
		log.Println("BootCurrent is not the latest installed kernel entry")
	}

	// If current boot is not latest, set latest to BootNext so it can
	// attempt to boot on next reboot
	//
	// Else, the current kernel booted successfully and the EFI variables
	// can be updated accordingly; notably, the BootCurrent will become
	// BootOrder[0]
	if !isCurrentBootLatest {
		if !*noBootNext {
			if err := km.SetLatestKernelToBootNext(); err != nil {
				log.Printf("Unable to set kernel fallback for new kernel: %v", err)
				os.Exit(1)
			}
			log.Println("Set kernel fallback mechanism for newly installed kernel")
		}
	} else {
		if err = km.CommitToBootLoader(); err != nil {
			log.Print(err)
			os.Exit(1)
		}
		// Cleanup old entries
		if err = km.RemoveObsoleteKernels(); err != nil {
			log.Print(err)
			os.Exit(1)
		}
		// This second call is intended to cleanup Boot variables that
		// become obsolete after the first call. It is not explicitly
		// tested, but I am not convinced that it is necessary. Regardless,
		// I do not want to break anything since I am not 100% sure of this
		if err = km.CommitToBootLoader(); err != nil {
			log.Print(err)
			os.Exit(1)
		}
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
	if jsonEfivars, ok := efivars.(*efibootmgr.MockEFIVariables); ok {
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
