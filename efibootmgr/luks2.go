// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"crypto/rand"
    "log"
    secboot "github.com/snapcore/secboot"
)

import "encoding/hex"
import "fmt"

func SetupRecoveryKey(devicePath string) error {
	fmt.Printf("SetupRecoveryKey: devicePath=%s\n", devicePath)

	unlockKeyNames, err := secboot.ListLUKS2ContainerUnlockKeyNames(devicePath)
	if err != nil {
		log.Printf("unlockKeyNames: %v items", len(unlockKeyNames))
		for v := range unlockKeyNames {
			log.Printf("unlock key name: %s", v)
		}
	} else  {
		log.Printf("error in ListLUKS2ContainerUnlockKeyNames")
	}

	prefix := "" // should default to "ubuntu-fde"
	//devicePath := "/dev/sda99"
	keyslotName := "???" // anything, stored in the token
	diskUnlockKey, err := secboot.GetDiskUnlockKeyFromKernel(prefix, devicePath, false) // deprecated
	// newer:
	container := secboot.FindStorageContainer(...)
	purpose := secboot.KeyringKeyPurposeUnlock
	secboot.GetKeyFromKernel(ctx, container, purpose, prefix)
	if err == nil {
	    log.Printf("Cannot get disk unlock key from kernel")
		return err
	}
	keyHex := make([]byte, hex.EncodedLen(len(diskUnlockKey)))
	hex.Encode(keyHex, diskUnlockKey[:])
	fmt.Printf("key: %s\n", keyHex)

    recoveryKey := secboot.RecoveryKey{} // generate random from crypto rand package
	rand.Read(recoveryKey[:])
	err = secboot.AddLUKS2ContainerRecoveryKey(devicePath, keyslotName, diskUnlockKey, recoveryKey)
    // soon to be replaced. (package secboot.luks2)
    if err != nil {
        log.Printf("error xxx")
    }
	// print recoveryKey

	return nil
}
