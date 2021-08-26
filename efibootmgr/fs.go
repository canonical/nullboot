// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"io"
	"os"
)

// FS abstracts away the filesystem.
//
// So we really wanted to use afero because it does all the magic for us, but it doubles
// our binary size, so that seems a tad much.
type FS interface {
	// Create behaves like os.Create()
	Create(path string) (io.WriteCloser, error)
	// Open behaves like os.Open()
	Open(path string) (io.ReadCloser, error)
	// ReadDir behaves like os.ReadDir()
	ReadDir(path string) ([]os.DirEntry, error)
}

// realFS implements FS using the os package
type realFS struct{}

func (realFS) Create(path string) (io.WriteCloser, error) { return os.Create(path) }
func (realFS) Open(path string) (io.ReadCloser, error)    { return os.Open(path) }
func (realFS) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }

// appFs is our default FS
var appFs FS = realFS{}
