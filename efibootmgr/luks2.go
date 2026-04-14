// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
    "log"
    secboot "github.com/snapcore/secboot"
)

func SetupRecoveryKey() error {
    log.Printf("setupRecoveryKey not implemented")
    keyslotName := "xxx"
	prefix := "undefined prefix"
	devicePath := "/dev/sda99"
	diskUnlockKey, err := secboot.GetDiskUnlockKeyFromKernel(prefix, devicePath, false)
	if err == nil {
	    log.Printf("Cannot get disk unlock key from kernel")
		return err
	}
    recoveryKey := secboot.RecoveryKey{} // generate random from crypto rand package
	err = secboot.AddLUKS2ContainerRecoveryKey(devicePath, keyslotName, diskUnlockKey, recoveryKey)
    // soon to be replaced. (package secboot.luks2)

    if err != nil {
        log.Printf("error xxx")

    }
    // print recoveryKey
	return nil
}
