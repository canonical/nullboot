// This file is part of nullboot
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

// Constants for LoadOption attributes
const (
	LoadOptionActive = 0x00000001
)

// LoadOption represents an EFI load option.
type LoadOption struct {
	Data []byte
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

// NewLoadOption binds efi_loadopt_create() in a Go-style fashion, it creates a load option.
// The returned load option's data can be set as a Boot variable.
func NewLoadOption(attributes uint32, dp DevicePath, desc string, optionalData []byte) (LoadOption, error) {
	var data []byte

	var optionalDataC *C.uchar
	if len(optionalData) != 0 {
		optionalDataC = (*C.uchar)(&optionalData[0])
	}

	needed := C.efi_loadopt_create(
		nil,
		0,
		C.uint32_t(attributes),
		(C.efidp)(unsafe.Pointer(&dp[0])),
		C.ssize_t(len(dp)),
		(*C.uchar)(unsafe.Pointer(C.CString(desc))),
		optionalDataC,
		C.size_t(len(optionalData)))

	if needed < 0 {
		return LoadOption{}, errors.New("Error occured in efi_loadopt_create sizing call")
	}

	data = make([]byte, needed)

	if C.efi_loadopt_create(
		(*C.uchar)(&data[0]),
		C.ssize_t(len(data)),
		C.uint32_t(attributes),
		(C.efidp)(unsafe.Pointer(&dp[0])),
		C.ssize_t(len(dp)),
		(*C.uchar)(unsafe.Pointer(C.CString(desc))),
		optionalDataC,
		C.size_t(len(optionalData))) != needed {
		return LoadOption{}, errors.New("Error occured in efi_loadopt_create final call")

	}

	return NewLoadOptionFromVariable(data)
}

// NewLoadOptionArgumentFromUTF8 converts a UTF-8 string into a UCS-2 encoded argument.
func NewLoadOptionArgumentFromUTF8(data string) ([]byte, error) {
	cData := (*C.uchar)(unsafe.Pointer(C.CString(data)))
	needed := C.efi_loadopt_args_as_ucs2(nil, 0, cData)
	if needed < 0 {
		return nil, errors.New("efi_loadopt_args_as_ucs2() returned -1 for string: " + data)
	}
	if needed == 0 {
		return nil, nil
	}
	buf := make([]byte, needed)
	if C.efi_loadopt_args_as_ucs2((*C.uint16_t)(unsafe.Pointer(&buf[0])), C.ssize_t(len(buf)), cData) < 0 {
		return nil, errors.New("efi_loadopt_args_as_ucs2() returned -1 for string: " + data)
	}
	return buf, nil
}

// Desc returns the description/label of a load option
func (lo LoadOption) Desc() string {
	clo := (*C.efi_load_option)(unsafe.Pointer(&lo.Data[0]))
	desc := C.efi_loadopt_desc(clo, C.ssize_t(len(lo.Data)))
	return C.GoString((*C.char)(unsafe.Pointer(desc)))
}

// Path returns the device path.
func (lo LoadOption) Path() DevicePath {
	clo := (*C.efi_load_option)(unsafe.Pointer(&lo.Data[0]))
	dp := C.efi_loadopt_path(clo, C.ssize_t(len(lo.Data)))
	return C.GoBytes(unsafe.Pointer(dp), C.int(C.efidp_size(dp)))
}
