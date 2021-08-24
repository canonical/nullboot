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

// LoadOption represents an EFI load option.
type LoadOption C.efi_load_option

// NewLoadOptionFromVariableUnsafe reinterprets the specified slice as a load option.
func NewLoadOptionFromVariableUnsafe(variable []byte) *LoadOption {
	return (*LoadOption)(unsafe.Pointer(&variable[0]))
}

// Desc returns the description/label of a load option
func (lo *LoadOption) Desc() string {
	clo := (*C.efi_load_option)(lo)
	desc := C.efi_loadopt_desc(clo, -1)
	return C.GoString((*C.char)(unsafe.Pointer(desc)))
}
