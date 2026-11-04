// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
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
	defaultCloudimgEncryptedDevice = "/dev/disk/by-label/" + efibootmgr.RootfsLabel
	defaultRecoveryName            = "recovery-0001"
)

// recoveryKeyOptions are the options accepted by the "recovery-key" command.
type recoveryKeyOptions struct {
	Create bool   `long:"create" description:"Create and set a recovery key"`
	List   bool   `long:"list" description:"List recovery keys"`
	Delete bool   `long:"delete" description:"Delete a recovery key"`
	Verify bool   `long:"verify" description:"Verify a recovery key"`
	Device string `long:"device" value-name:"<device>" description:"Device of the encrypted volume"`
	Name   string `long:"name" value-name:"<name>" description:"Name of the recovery key"`
}

var defaultRecoveryKeyOptions = recoveryKeyOptions{
	Device: defaultCloudimgEncryptedDevice,
	Name:   defaultRecoveryName,
}

func cmdRecoveryKey(opts *recoveryKeyOptions) {
	mutuallyExclusiveOptsCount := 0
	if opts.Create {
		mutuallyExclusiveOptsCount++
	}
	if opts.Delete {
		mutuallyExclusiveOptsCount++
	}
	if opts.List {
		mutuallyExclusiveOptsCount++
	}
	if opts.Verify {
		mutuallyExclusiveOptsCount++
	}

	if mutuallyExclusiveOptsCount > 1 {
		fmt.Fprintf(os.Stderr, "Options cannot be used together: --create, --list, --delete, --verify\n")
		os.Exit(1)
	}

	var err error
	if opts.List {
		err = luks2.ListRecoveryKeys(opts.Device)
	} else if opts.Delete {
		err = luks2.DeleteRecoveryKey(opts.Device, opts.Name)
	} else if opts.Create {
		err = luks2.CreateRecoveryKey(opts.Device, opts.Name)
	} else if opts.Verify {
		err = luks2.VerifyRecoveryKey(opts.Device)
	} else {
		fmt.Fprintf(os.Stderr, "Please select at least one action: --create, --list, --delete, --verify\n")
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
}
