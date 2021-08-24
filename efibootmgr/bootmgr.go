// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

// Package efibootmgr contains a boot management library
package efibootmgr

import (
	"fmt"
	"log"
)

import "github.com/canonical/bootmgrless/efivars"

const (
	maxBootEntries = 65535 // Maximum number of boot entries we can hold
)

// BootEntryVariable defines a boot entry variable
type BootEntryVariable struct {
	BootNumber int                // number of the Boot variable, for example, for Boot0004 this is 4
	Data       []byte             // the data of the variable
	Attributes uint32             // any attributes set on the variable
	LoadOption efivars.LoadOption // the data of the variable parsed as a load option, if it is a valid load option
}

// BootManager manages the boot device selection menu entries (Boot0000...BootFFFF).
type BootManager struct {
	entries   map[int]BootEntryVariable // The Boot<number> variables
	bootOrder []int                     // The BootOrder variable, parsed
}

// NewBootManagerFromSystem returns a new BootManager object, initialized with the system state.
func NewBootManagerFromSystem() BootManager {
	var err error
	bm := BootManager{make(map[int]BootEntryVariable), nil}

	bootOrderBytes, _ := efivars.GetVariable(efivars.GUIDGlobal, "BootOrder")
	bm.bootOrder = make([]int, len(bootOrderBytes)/2)
	for i := 0; i < len(bootOrderBytes); i += 2 {
		// FIXME: It's probably not valid to assume little-endian here?
		bm.bootOrder[i/2] = int(bootOrderBytes[i+1])<<16 + int(bootOrderBytes[i])
	}

	for ok, guid, name := efivars.GetNextVariable(); ok; ok, guid, name = efivars.GetNextVariable() {
		var entry BootEntryVariable
		// Boot entries are stored with the global GUID
		if *guid != efivars.GUIDGlobal {
			continue
		}
		if parsed, err := fmt.Sscanf(name, "Boot%04X", &entry.BootNumber); len(name) != 8 || parsed != 1 || err != nil {
			continue
		}
		entry.Data, entry.Attributes = efivars.GetVariable(*guid, name)
		entry.LoadOption, err = efivars.NewLoadOptionFromVariable(entry.Data)
		if err != nil {
			log.Printf("Invalid boot entry Boot%04X: %s\n", entry.BootNumber, err)
		}

		bm.entries[entry.BootNumber] = entry
	}

	return bm
}

// NextFreeEntry returns the number of the next free Boot variable.
func (bm *BootManager) NextFreeEntry() (int, error) {
	for i := 0; i < maxBootEntries; i++ {
		if _, ok := bm.entries[i]; !ok {
			return i, nil
		}
	}

	return -1, fmt.Errorf("Maximum number of boot entries exceeded")
}

// AddEntry adds a boot entry to the boot manager.
// This finds the next free variable and adds a boot entry
func (bm *BootManager) AddEntry(desc string, path string, options []string) (int, error) {
	bootNext, err := bm.NextFreeEntry()
	if err != nil {
		return -1, err
	}
	variable := fmt.Sprintf("Boot%04X", bootNext)

	// FIXME: AddEntry is a stub
	log.Printf("Adding boot entry %v", variable)

	// FIXME: Fill in Data, Attributes, and LoadOption that we will store above
	bm.entries[bootNext] = BootEntryVariable{
		BootNumber: bootNext,
	}

	return bootNext, nil
}

// DeleteEntry deletes an entry.
func (bm *BootManager) DeleteEntry(bootNum int) error {
	// FIXME: DeleteEntry is a stub
	return fmt.Errorf("Deleting is not yet implemented")
}
