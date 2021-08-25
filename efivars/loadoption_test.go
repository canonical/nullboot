// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efivars

import (
	"bytes"
	"reflect"
	"regexp"
	"testing"
)

var UsbrBootCdrom = []byte{9, 0, 0, 0, 28, 0, 85, 0, 83, 0, 66, 0, 82, 0, 32, 0, 66, 0, 79, 0, 79, 0, 84, 0, 32, 0, 67, 0, 68, 0, 82, 0, 79, 0, 77, 0, 0, 0, 2, 1, 12, 0, 208, 65, 3, 10, 0, 0, 0, 0, 1, 1, 6, 0, 0, 20, 3, 5, 6, 0, 11, 1, 127, 255, 4, 0}

func TestNewLoadOptionFromVariable(t *testing.T) {
	tests := []struct {
		label string
		input []byte
		want  LoadOption
		err   string
	}{
		{"USBR Boot CDROM", UsbrBootCdrom, LoadOption{UsbrBootCdrom}, ""},
		{"Dummy {0,1}", []byte{0, 1}, LoadOption{}, "Invalid load option"},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			got, err := NewLoadOptionFromVariable(tc.input)
			if tc.err != "" {
				if matched, _ := regexp.MatchString(tc.err, err.Error()); !matched {
					t.Fatalf("expected error: %v, got: %v", tc.err, err)
				}
			}
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected: %v, got: %v", tc.want, got)
			}
		})

	}
}

func TestLoadOptionDesc(t *testing.T) {
	tests := []struct {
		label string
		input []byte
		want  string
	}{
		{"USBR Boot CDROM", UsbrBootCdrom, "USBR BOOT CDROM"},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			lo, _ := NewLoadOptionFromVariable(tc.input)
			got := lo.Desc()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected: %v, got: %v", tc.want, got)
			}
		})

	}
}

func TestLoadOptionPath(t *testing.T) {
	tests := []struct {
		label string
		input []byte
		want  []byte
	}{
		{"USBR Boot CDROM", UsbrBootCdrom, UsbrBootCdrom[38:]},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			lo, _ := NewLoadOptionFromVariable(tc.input)
			got := lo.Path()
			if !bytes.Equal(tc.want, got) {
				t.Fatalf("\n"+
					"expected: %v\n"+
					"got:      %v", tc.want, got)
			}
		})

	}
}

func TestNewLoadOptionArgumentFromUTF8(t *testing.T) {
	tests := []struct {
		input string
		want  []byte
		err   error
	}{
		{"ubuntu", []byte{'u', 0, 'b', 0, 'u', 0, 'n', 0, 't', 0, 'u', 0}, nil},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := NewLoadOptionArgumentFromUTF8(tc.input)
			if tc.err != err {
				t.Fatalf("expected error: %v, got: %v", tc.err, err)
			}
			if !bytes.Equal(tc.want, got) {
				t.Fatalf("expected: %v, got: %v", tc.want, got)
			}
		})

	}
}
