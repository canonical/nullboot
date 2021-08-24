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
import "errors"

// LoadOption represents an EFI load option.
type LoadOption struct {
	data []byte
}

// NewLoadOptionFromVariable reinterprets the specified slice as a load option.
func NewLoadOptionFromVariable(variable []byte) (LoadOption, error) {
	if variable == nil {
		panic("nil value")
	}
	clo := (*C.efi_load_option)(unsafe.Pointer(&variable[0]))
	if len(variable) == 0 || C.efi_loadopt_is_valid(clo, C.size_t(len(variable))) == 0 {
		return LoadOption{}, errors.New("Invalid load option")
	}

	return LoadOption{variable}, nil
}

// Desc returns the description/label of a load option
func (lo *LoadOption) Desc() string {
	clo := (*C.efi_load_option)(unsafe.Pointer(&lo.data[0]))
	desc := C.efi_loadopt_desc(clo, C.ssize_t(len(lo.data)))
	return C.GoString((*C.char)(unsafe.Pointer(desc)))
}
