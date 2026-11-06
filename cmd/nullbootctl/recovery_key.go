// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"github.com/canonical/nullboot/efibootmgr"
	"github.com/canonical/nullboot/luks2"
	"os"
)

const UsageRecoveryKey = `Usage:
nullbootctl recovery-key --create [--device <device>] [--name <name>]
                         --delete [--device <device>] [--name <name>]
                         --list   [--device <device>]
                         --verify [--device <device>]

Options:
  <device>   LUKS2 device
  <name>     Name of the recovery key
`

const (
	default_cloudimg_encrypted_device = "/dev/disk/by-label/" + efibootmgr.RootfsLabel
	default_recovery_name             = "recovery-0001"
)

func usageRecoveryKey() {
	fmt.Print(UsageRecoveryKey)
}

func cmdRecoveryKey() {
	var devicePath string
	var recoveryName string
	doCreate := flag.Bool("create", false, "Create and set a recovery key")
	doList := flag.Bool("list", false, "List recovery keys")
	doDelete := flag.Bool("delete", false, "Delete a recovery key")
	doVerify := flag.Bool("verify", false, "Verify a recovery key")
	flag.StringVar(&devicePath, "device", default_cloudimg_encrypted_device, "Device of the encrypted volume")
	flag.StringVar(&recoveryName, "name", default_recovery_name, "Name of the recovery key")
	flag.Usage = usageRecoveryKey
	flag.Parse()

	mutuallyExclusiveOptsCount := 0
	if *doCreate {
		mutuallyExclusiveOptsCount++
	}
	if *doDelete {
		mutuallyExclusiveOptsCount++
	}
	if *doList {
		mutuallyExclusiveOptsCount++
	}
	if *doVerify {
		mutuallyExclusiveOptsCount++
	}

	if mutuallyExclusiveOptsCount > 1 {
		fmt.Fprintf(os.Stderr, "Options cannot be used together: --create, --list, --delete, --verify\n")
		os.Exit(1)
	}

	var err error
	if *doList {
		err = luks2.ListRecoveryKeys(devicePath)
	} else if *doDelete {
		err = luks2.DeleteRecoveryKey(devicePath, recoveryName)
	} else if *doCreate {
		err = luks2.CreateRecoveryKey(devicePath, recoveryName)
	} else if *doVerify {
		err = luks2.VerifyRecoveryKey(devicePath)
	} else {
		fmt.Fprintf(os.Stderr, "Please select at least one action: --create, --list, --delete, --verify\n")
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
}
