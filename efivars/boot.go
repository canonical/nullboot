// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efivars

/*
#include <efivar/efiboot.h>
#cgo LDFLAGS: -lefivar -lefiboot

// Wrapper for efi_generate_file_device_path without varargs
ssize_t go_efi_generate_file_device_path(uint8_t *buf, ssize_t size, const char * const filepath, uint32_t options ) {
    return efi_generate_file_device_path(buf, size, filepath, options);
}
*/
import "C"
import "unsafe"
import "errors"

// Various options for GenerateFileDevicePath.
// We ignore the Edd10 abbreviation option, as it requires a vaarg.
const (
	BootAbbrevNone            = C.EFIBOOT_ABBREV_NONE             // Do not abbreviate things
	BootAbbrevHD              = C.EFIBOOT_ABBREV_HD               // Abbreviate HD
	BootAbbrevFile            = C.EFIBOOT_ABBREV_FILE             // Abbreviate file path
	BootOptionWriteSignature  = C.EFIBOOT_OPTIONS_WRITE_SIGNATURE // Write MBR signature to boot partition
	BootOptionIgnorePMBRError = C.EFIBOOT_OPTIONS_IGNORE_PMBR_ERR // Ignore PBMR error
)

// DevicePath represents a device path.
type DevicePath []byte

// NewDevicePath generates a UEFI device file path from a given real file path.
// It returns it as a slice of bytes which can later be parsed into a DevicePath object.
//
func NewDevicePath(filepath string, options uint32) (DevicePath, error) {
	// Gather the size we need first
	size := C.go_efi_generate_file_device_path((*C.uchar)(unsafe.Pointer(nil)),
		C.ssize_t(0), C.CString(filepath),
		C.uint32_t(options))
	if size < 0 {
		return nil, errors.New("Could not generate file device path")
	}
	buf := make([]byte, size)
	// Now actually build the device path into the buffer we just allocated
	size = C.go_efi_generate_file_device_path((*C.uchar)(unsafe.Pointer(&buf[0])),
		C.ssize_t(size),
		C.CString(filepath),
		C.uint32_t(options))
	return buf, nil
}
