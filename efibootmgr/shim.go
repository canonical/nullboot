// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"fmt"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"io"
	"os"
	"strings"
)

// BootEntry is a boot entry.
type BootEntry struct {
	filename    string
	label       string
	options     string
	description string
}

// WriteShimFallbackToFile opens the specified path in UTF-16LE and then calls WriteShimFallback
func WriteShimFallbackToFile(path string, entries []BootEntry) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", path, err)
	}

	writer := transform.NewWriter(file, unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder())
	return WriteShimFallback(writer, entries)
}

// WriteShimFallback writes out a BOOT*.CSV for the shim fallback loader to the specified writer.
// The output of this function is unencoded, use a transformed UTF-16 writer.
func WriteShimFallback(w io.Writer, entries []BootEntry) error {
	for _, entry := range entries {
		if strings.Contains(entry.filename, ",") ||
			strings.Contains(entry.label, ",") ||
			strings.Contains(entry.options, ",") ||
			strings.Contains(entry.description, ",") {
			return fmt.Errorf("entry '%s' contains ',' in one of the attributes, this is not supported", entry.label)
		}

		_, err := fmt.Fprintf(w, "%s,%s,%s,%s\n", entry.filename, entry.label, entry.options, entry.description)
		if err != nil {
			return fmt.Errorf("Could not write entry '%s' to file: %w", entry.label, err)
		}
	}

	return nil
}