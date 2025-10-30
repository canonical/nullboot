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
	efi_linux "github.com/canonical/go-efilib/linux"
)

const (
	maxBootEntries = 65535 // Maximum number of boot entries we can hold
)

// BootEntryVariable defines a boot entry variable
type BootEntryVariable struct {
	BootNumber int                    // number of the Boot variable, for example, for Boot0004 this is 4
	Data       []byte                 // the data of the variable
	Attributes efi.VariableAttributes // any attributes set on the variable
	LoadOption *efi.LoadOption        // the data of the variable parsed as a load option, if it is a valid load option
}

// BootManager manages the boot device selection menu entries (Boot0000...BootFFFF).
type BootManager struct {
	efivars        EFIVariables              // EFIVariables implementation
	entries        map[int]BootEntryVariable // The Boot<number> variables
	bootOrder      []int                     // The BootOrder variable, parsed
	bootOrderAttrs efi.VariableAttributes    // The attributes of BootOrder variable
}

// NewBootManagerFromSystem returns a new BootManager object, initialized with the system state.
func NewBootManagerFromSystem() (BootManager, error) {
	return NewBootManagerForVariables(RealEFIVariables{})
}

// NewBootManagerForVariables returns a boot manager for the given EFIVariables manager
func NewBootManagerForVariables(efivars EFIVariables) (BootManager, error) {
	var err error
	bm := BootManager{}
	bm.efivars = efivars

	if !VariablesSupported(efivars) {
		return BootManager{}, fmt.Errorf("Variables not supported")
	}

	bootOrderBytes, bootOrderAttrs, err := bm.efivars.GetVariable(efi.GlobalVariable, "BootOrder")
	if err != nil {
		log.Println("Could not read BootOrder variable, populating with default, error was:", err)
		bootOrderBytes = nil
		bootOrderAttrs = efi.AttributeNonVolatile | efi.AttributeBootserviceAccess | efi.AttributeRuntimeAccess
	}
	bm.bootOrder = make([]int, len(bootOrderBytes)/2)
	bm.bootOrderAttrs = bootOrderAttrs
	for i := 0; i < len(bootOrderBytes); i += 2 {
		// FIXME: It's probably not valid to assume little-endian here?
		bm.bootOrder[i/2] = int(binary.LittleEndian.Uint16(bootOrderBytes[i : i+2]))
	}

	bm.entries = make(map[int]BootEntryVariable)
	names, err := GetVariableNames(bm.efivars, efi.GlobalVariable)
	if err != nil {
		return BootManager{}, fmt.Errorf("cannot obtain list of global variables: %v", err)
	}
	for _, name := range names {
		var entry BootEntryVariable
		if parsed, err := fmt.Sscanf(name, "Boot%04X", &entry.BootNumber); len(name) != 8 || parsed != 1 || err != nil {
			continue
		}
		entry.Data, entry.Attributes, err = bm.efivars.GetVariable(efi.GlobalVariable, name)
		if err != nil {
			return BootManager{}, fmt.Errorf("cannot read %s: %v", name, err)
		}
		entry.LoadOption, err = efi.ReadLoadOption(bytes.NewReader(entry.Data))
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
	// Try to find entry
	bootEntryVar, err := bm.FindBootEntryVar(&entry, relativeTo)
	if err != nil {
		return -1, fmt.Errorf("Unable to determine if entry exists: %v", err)
	}

	// Entry was found; return
	if bootEntryVar != nil {
		return bootEntryVar.BootNumber, nil
	}

	// Entry was not found; generate requisite data
	dp, err := bm.efivars.NewFileDevicePath(path.Join(relativeTo, entry.Filename), efi_linux.ShortFormPathHD)
	if err != nil {
		return -1, fmt.Errorf("Unable to derive device path: %v", err)
	}
	bootNext, err := bm.NextFreeEntry()
	if err != nil {
		return -1, fmt.Errorf("Unable to generate a new boot number: %v", err)
	}

	// Create Entry Variable
	entryVar, err := CreateEntryVar(&entry, bootNext, dp)
	if err != nil {
		return -1, fmt.Errorf("Unable to create the boot entry variable: %v", err)
	}

	// Set system boot variable for new entry
	err = bm.SetBootEntryVariable(entryVar)
	if err != nil {
		return entryVar.BootNumber, fmt.Errorf("Unable to set environment variables for Boot%04X: %v", entryVar.BootNumber, err)
	}
	return entryVar.BootNumber, nil
}

// FindBootEntryVar finds a matching entry in the boot device selection menu
// associated to the input BootEntry in a relative directory.
//
// It returns the entry variable, or nil if not found, with error set where
// applicable.
//
// The argument relativeTo specifies the directory entry.Filename is in.
func (bm *BootManager) FindBootEntryVar(entry *BootEntry, relativeTo string) (*BootEntryVariable, error) {
	dp, err := bm.efivars.NewFileDevicePath(path.Join(relativeTo, entry.Filename), efi_linux.ShortFormPathHD)
	if err != nil {
		return nil, fmt.Errorf("Unable to derive device path: %v", err)
	}
	loadOption := getLoadOption(entry, dp)
	data, err := loadOption.Bytes()
	if err != nil {
		return nil, fmt.Errorf("Unable to encode load option for %s: %v", entry.Label, err)
	}
	attrib := defaultAttrib()
	return findBootEntryVar(bm.entries, data, attrib), nil
}

// SetBootEntryVariable creates a system boot variable from the provided
// BootEntryVariable.
//
// It returns an error should there be an issue setting the system variable.
func (bm *BootManager) SetBootEntryVariable(entryVar *BootEntryVariable) error {
	variable := fmt.Sprintf("Boot%04X", entryVar.BootNumber)

	if err := bm.efivars.SetVariable(efi.GlobalVariable, variable, entryVar.Data, entryVar.Attributes); err != nil {
		return err
	}
	bm.entries[entryVar.BootNumber] = *entryVar
	return nil
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

	if err := DelVariable(bm.efivars, efi.GlobalVariable, variable); err != nil {
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
	if err := bm.efivars.SetVariable(efi.GlobalVariable, "BootOrder", output, bm.bootOrderAttrs); err != nil {
		return err
	}

	bm.bootOrder = newOrder
	return nil

}

// getLoadOption derives a standardized LoadOption from a specified entry
// and device path.
//
// It returns a pointer to the new LoadOption.
func getLoadOption(entry *BootEntry, dp efi.DevicePath) *efi.LoadOption {
	optionalData := GetEntryVarData(entry)
	loadoption := &efi.LoadOption{
		Attributes:   efi.LoadOptionActive,
		Description:  entry.Label,
		FilePath:     dp,
		OptionalData: optionalData}

	return loadoption
}

// CreateEntryVar creates a BootEntryVariable derived from a BootEntry,
// given a boot number and device path.
//
// Returns a pointer to a BootEntryVariable if the operation is successful
// or otherwise, returns nil and an error.
func CreateEntryVar(entry *BootEntry, bootNum int, dp efi.DevicePath) (*BootEntryVariable, error) {
	loadoption := getLoadOption(entry, dp)
	data, err := loadoption.Bytes()
	if err != nil {
		return nil, fmt.Errorf("cannot encode boot entry data: %v", err)
	}
	attrib := defaultAttrib()

	entryVar := BootEntryVariable{
		BootNumber: bootNum,
		Data:       data,
		Attributes: attrib,
		LoadOption: loadoption,
	}

	return &entryVar, nil
}

// GetEntryVarData standardizes the collection of data from a BootEntry in
// the form of a BootEntryVariable's Data field.
//
// Returns a slice of LittleEndian, UCS2 formatted bytes.
func GetEntryVarData(entry *BootEntry) []byte {
	optionalData := new(bytes.Buffer)
	binary.Write(optionalData, binary.LittleEndian, efi.ConvertUTF8ToUCS2(entry.Options+"\x00"))
	return optionalData.Bytes()
}

// findBootEntryVar determines whether the given data and attributes match
// any BootEntryVariable in a map of BootEntryVariables
//
// Returns the pointer to the BootEntryVariable or nil if there is no
// match.
func findBootEntryVar(entryVars map[int]BootEntryVariable, data []byte, attrib efi.VariableAttributes) *BootEntryVariable {
	for _, existingVar := range entryVars {
		if bytes.Equal(existingVar.Data, data) && existingVar.Attributes == attrib {
			return &existingVar
		}
	}
	return nil
}

// defaultAttrib generates a set of standard attributes for boot variables
func defaultAttrib() efi.VariableAttributes {
	return efi.AttributeNonVolatile | efi.AttributeBootserviceAccess | efi.AttributeRuntimeAccess
}
