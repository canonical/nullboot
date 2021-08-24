// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efivars

/*
#include <efivar/efiboot.h>
#cgo LDFLAGS: -lefivar -lefiboot
*/
import "C"
import "unsafe"

// GUID of a variable
type GUID = C.efi_guid_t

// GetNextVariable returns the next variable based on the passed arguments.
// If it returns true, the variables have all been iterated over.
func GetNextVariable() (bool, *GUID, string) {
	var guid *C.efi_guid_t
	var name *C.char
	if C.efi_get_next_variable_name(&guid, &name) != 0 {
		return true, guid, C.GoString(name)
	}

	return false, nil, ""
}

// VariablesSupported returns if variables are supported
func VariablesSupported() bool {
	return C.efi_variables_supported() != 0
}

// GetVariable retrieves the content of the specified variable.
// It returns the content and the attributes
func GetVariable(guid GUID, name string) (data []byte, attrs uint32) {
	var size C.size_t
	var attributes C.uint32_t
	var rawData *C.uchar
	if C.efi_get_variable(guid, C.CString(name), &rawData, &size, &attributes) != 0 {
		return nil, 0
	}

	return C.GoBytes(unsafe.Pointer(rawData), C.int(size)), uint32(attributes)
}
