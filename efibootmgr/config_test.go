// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"reflect"
	"testing"

	"github.com/spf13/afero"
)

func TestReadConfig(t *testing.T) {
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}

	config, err := ReadConfig("/etc/nullboot.conf")
	if err != nil {
		t.Fatalf("unexpected error reading missing configuration: %v", err)
	}
	if len(config.KernelPriority) != 0 {
		t.Errorf("expected no priorities for missing file, got %v", config.KernelPriority)
	}
	if got := config.weight("6.8.0-52-fips"); got != 0 {
		t.Errorf("expected weight 0 without configuration, got %d", got)
	}

	if err := afero.WriteFile(memFs, "/etc/nullboot.conf", []byte(`# Prefer FIPS kernels over Azure FDE kernels
kernel-priority:
  fips: 1000
  azure-fde: 100
`), 0644); err != nil {
		t.Fatal(err)
	}

	config, err = ReadConfig("/etc/nullboot.conf")
	if err != nil {
		t.Fatalf("unexpected error reading configuration: %v", err)
	}
	want := map[string]int{"fips": 1000, "azure-fde": 100}
	if !reflect.DeepEqual(config.KernelPriority, want) {
		t.Errorf("unexpected kernel priorities: %v", config.KernelPriority)
	}
}

func TestConfigWeight(t *testing.T) {
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	if err := afero.WriteFile(memFs, "/etc/nullboot.conf", []byte(`kernel-priority:
  fips: 1000
  azure-fde: 100
`), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := ReadConfig("/etc/nullboot.conf")
	if err != nil {
		t.Fatalf("unexpected error reading configuration: %v", err)
	}

	tests := []struct {
		abi  string
		want int
	}{
		{"6.8.0-52-fips", 1000},
		{"6.8.0-52-azure-fde", 100}, // multi-dash flavour
		{"6.8.0-1003-azure-fde", 100},
		{"6.8.0-52-generic", 0}, // unlisted flavour has no priority
		{"6.8.0-52-lowlatency", 0},
	}
	for _, tt := range tests {
		if got := config.weight(tt.abi); got != tt.want {
			t.Errorf("weight(%q): expected %d, got %d", tt.abi, tt.want, got)
		}
	}
}

func TestReadConfigDropIns(t *testing.T) {
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}

	for name, content := range map[string]string{
		"/etc/nullboot.conf":                      "kernel-priority: {fips: 1000, azure-fde: 100}\n",
		"/etc/nullboot.conf.d/10-override.conf":   "kernel-priority: {fips: 500}\n",
		"/etc/nullboot.conf.d/20-lowlatency.conf": "kernel-priority: {lowlatency: 10}\n",
		"/etc/nullboot.conf.d/ignored.txt":        "kernel-priority: {generic: 1}\n",
	} {
		if err := afero.WriteFile(memFs, name, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	config, err := ReadConfig("/etc/nullboot.conf")
	if err != nil {
		t.Fatalf("unexpected error reading configuration: %v", err)
	}
	want := map[string]int{"fips": 500, "azure-fde": 100, "lowlatency": 10}
	if !reflect.DeepEqual(config.KernelPriority, want) {
		t.Errorf("expected kernel priorities %v, got %v", want, config.KernelPriority)
	}
}

func TestReadConfigInvalidYAML(t *testing.T) {
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	if err := afero.WriteFile(memFs, "/etc/nullboot.conf", []byte("kernel-priority: [not a map"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadConfig("/etc/nullboot.conf"); err == nil {
		t.Error("expected error for malformed YAML, got nil")
	}
}
