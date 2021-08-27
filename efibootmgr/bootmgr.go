// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

// Package efibootmgr contains a boot management library
package efibootmgr

import (
	"bytes"
	"fmt"
	"log"
)

import "github.com/canonical/nullboot/efivars"

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
	efivars   EFIVariables              // An implementation of efi variables
}

// NewBootManagerFromSystem returns a new BootManager object, initialized with the system state.
func NewBootManagerFromSystem() (BootManager, error) {
	return newBootManagerFromVariables(RealEFIVariables{})
}

// newBootManagerFromVariables abstraction
func newBootManagerFromVariables(vars EFIVariables) (BootManager, error) {
	var err error
	bm := BootManager{make(map[int]BootEntryVariable), nil, vars}

	if !bm.efivars.VariablesSupported() {
		return BootManager{}, fmt.Errorf("Variables not supported")
	}

	bootOrderBytes, _ := bm.efivars.GetVariable(efivars.GUIDGlobal, "BootOrder")
	bm.bootOrder = make([]int, len(bootOrderBytes)/2)
	for i := 0; i < len(bootOrderBytes); i += 2 {
		// FIXME: It's probably not valid to assume little-endian here?
		bm.bootOrder[i/2] = int(bootOrderBytes[i+1])<<16 + int(bootOrderBytes[i])
	}

	for _, name := range bm.efivars.GetVariableNames(efivars.GUIDGlobal) {
		var entry BootEntryVariable
		if parsed, err := fmt.Sscanf(name, "Boot%04X", &entry.BootNumber); len(name) != 8 || parsed != 1 || err != nil {
			continue
		}
		entry.Data, entry.Attributes = bm.efivars.GetVariable(efivars.GUIDGlobal, name)
		entry.LoadOption, err = efivars.NewLoadOptionFromVariable(entry.Data)
		if err != nil {
			log.Printf("Invalid boot entry Boot%04X: %s\n", entry.BootNumber, err)
		}

		bm.entries[entry.BootNumber] = entry
	}

	return bm, nil
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

// FindOrCreateEntry finds a matching entry in the boot device selection menu,
// or creates one if it is missing.
//
// It returns the number of the entry created, or -1 on failure, with error set.
func (bm *BootManager) FindOrCreateEntry(entry BootEntry) (int, error) {
	bootNext, err := bm.NextFreeEntry()
	if err != nil {
		return -1, err
	}
	variable := fmt.Sprintf("Boot%04X", bootNext)

	dp, err := bm.efivars.NewDevicePath(entry.Filename, efivars.BootAbbrevHD)
	if err != nil {
		return -1, err
	}

	optionalData, err := efivars.NewLoadOptionArgumentFromUTF8(entry.Options)
	optionalData = append(optionalData, 0)

	if err != nil {
		return -1, err
	}

	loadoption, err := efivars.NewLoadOption(efivars.LoadOptionActive, dp, entry.Label, optionalData)
	if err != nil {
		return -1, err
	}

	entryVar := BootEntryVariable{
		BootNumber: bootNext,
		Data:       loadoption.Data,
		Attributes: efivars.VariableNonVolatile | efivars.VariableBootServiceAccess | efivars.VariableRuntimeAccess,
		LoadOption: loadoption,
	}

	// Detect duplicates and ignore
	for _, existingVar := range bm.entries {
		if bytes.Equal(existingVar.LoadOption.Data, loadoption.Data) && existingVar.Attributes == entryVar.Attributes {
			return existingVar.BootNumber, nil
		}
	}

	if err := bm.efivars.SetVariable(efivars.GUIDGlobal, variable, entryVar.Data, entryVar.Attributes, 0644); err != nil {
		return -1, nil
	}

	bm.entries[bootNext] = entryVar

	return bootNext, nil
}

// DeleteEntry deletes an entry.
func (bm *BootManager) DeleteEntry(bootNum int) error {
	// FIXME: DeleteEntry is a stub
	return fmt.Errorf("Deleting is not yet implemented")
}
