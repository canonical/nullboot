// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

// Package efibootmgr contains a boot management library
package efibootmgr

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"path"

	"github.com/canonical/go-efilib"

	"github.com/canonical/nullboot/efivars"
)

const (
	maxBootEntries = 65535 // Maximum number of boot entries we can hold
)

// BootEntryVariable defines a boot entry variable
type BootEntryVariable struct {
	BootNumber int                    // number of the Boot variable, for example, for Boot0004 this is 4
	Data       []byte                 // the data of the variable
	Attributes efi.VariableAttributes // any attributes set on the variable
	LoadOption efivars.LoadOption     // the data of the variable parsed as a load option, if it is a valid load option
}

// BootManager manages the boot device selection menu entries (Boot0000...BootFFFF).
type BootManager struct {
	entries        map[int]BootEntryVariable // The Boot<number> variables
	bootOrder      []int                     // The BootOrder variable, parsed
	bootOrderAttrs efi.VariableAttributes    // The attributes of BootOrder variable
}

// NewBootManagerFromSystem returns a new BootManager object, initialized with the system state.
func NewBootManagerFromSystem() (BootManager, error) {
	var err error
	bm := BootManager{}

	if !VariablesSupported() {
		return BootManager{}, fmt.Errorf("Variables not supported")
	}

	bootOrderBytes, bootOrderAttrs, err := GetVariable(efi.GlobalVariable, "BootOrder")
	if err != nil {
		return BootManager{}, fmt.Errorf("cannot read BootOrder variable: %v", err)
	}
	bm.bootOrder = make([]int, len(bootOrderBytes)/2)
	bm.bootOrderAttrs = bootOrderAttrs
	for i := 0; i < len(bootOrderBytes); i += 2 {
		// FIXME: It's probably not valid to assume little-endian here?
		bm.bootOrder[i/2] = int(binary.LittleEndian.Uint16(bootOrderBytes[i : i+2]))
	}

	bm.entries = make(map[int]BootEntryVariable)
	names, err := GetVariableNames(efi.GlobalVariable)
	if err != nil {
		return BootManager{}, fmt.Errorf("cannot obtain list of global variables: %v", err)
	}
	fmt.Println("names:", names)
	for _, name := range names {
		var entry BootEntryVariable
		if parsed, err := fmt.Sscanf(name, "Boot%04X", &entry.BootNumber); len(name) != 8 || parsed != 1 || err != nil {
			continue
		}
		fmt.Println(" reading:", name)
		entry.Data, entry.Attributes, err = GetVariable(efi.GlobalVariable, name)
		if err != nil {
			return BootManager{}, fmt.Errorf("cannot read %s: %v", name, err)
		}
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
//
// The argument relativeTo specifies the directory entry.Filename is in.
func (bm *BootManager) FindOrCreateEntry(entry BootEntry, relativeTo string) (int, error) {
	bootNext, err := bm.NextFreeEntry()
	if err != nil {
		return -1, err
	}
	variable := fmt.Sprintf("Boot%04X", bootNext)

	dp, err := appEFIVars.NewDevicePath(path.Join(relativeTo, entry.Filename), efivars.BootAbbrevHD)
	if err != nil {
		return -1, err
	}

	optionalData, err := efivars.NewLoadOptionArgumentFromUTF8(entry.Options)
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
		Attributes: efi.AttributeNonVolatile | efi.AttributeBootserviceAccess | efi.AttributeRuntimeAccess,
		LoadOption: loadoption,
	}

	// Detect duplicates and ignore
	for _, existingVar := range bm.entries {
		if bytes.Equal(existingVar.LoadOption.Data, loadoption.Data) && existingVar.Attributes == entryVar.Attributes {
			return existingVar.BootNumber, nil
		}
	}

	if err := SetVariable(efi.GlobalVariable, variable, entryVar.Data, entryVar.Attributes); err != nil {
		return -1, err
	}

	bm.entries[bootNext] = entryVar

	return bootNext, nil
}

// DeleteEntry deletes an entry and updates the cached boot order.
//
// The boot order still needs to be committed afterwards. It is not written back immediately,
// as there will usually be multiple places to update boot order, and we can coalesce those
// writes. We still have to update the boot order though, such that when we delete an entry
// and then create a new one with the same number we don't accidentally have the new one in
// the order.
func (bm *BootManager) DeleteEntry(bootNum int) error {
	variable := fmt.Sprintf("Boot%04X", bootNum)
	if _, ok := bm.entries[bootNum]; !ok {
		return fmt.Errorf("Tried deleting a non-existing variable %s", variable)
	}

	if err := DelVariable(efi.GlobalVariable, variable); err != nil {
		return err
	}
	delete(bm.entries, bootNum)

	var newOrder []int

	for _, orderEntry := range bm.bootOrder {
		if orderEntry != bootNum {
			newOrder = append(newOrder, orderEntry)
		}

	}

	bm.bootOrder = newOrder

	return nil
}

// PrependAndSetBootOrder commits a new boot order or returns an error.
//
// The boot order specified is prepended to the existing one, and the order
// is deduplicated before committing.
func (bm *BootManager) PrependAndSetBootOrder(head []int) error {
	var newOrder []int

	// Combine head with existing boot order, filter out duplicates and non-existing entries
	for _, num := range append(append([]int(nil), head...), bm.bootOrder...) {
		isDuplicate := false
		for _, otherNum := range newOrder {
			if otherNum == num {
				isDuplicate = true
			}
		}
		if _, ok := bm.entries[num]; ok && !isDuplicate {
			newOrder = append(newOrder, num)
		}
	}

	// Encode the boot order to bytes
	var output []byte
	for _, num := range newOrder {
		var numBytes [2]byte
		binary.LittleEndian.PutUint16(numBytes[0:], uint16(num))
		output = append(output, numBytes[0], numBytes[1])
	}

	// Set the boot order and update our cache
	if err := SetVariable(efi.GlobalVariable, "BootOrder", output, bm.bootOrderAttrs); err != nil {
		return err
	}

	bm.bootOrder = newOrder
	return nil

}
