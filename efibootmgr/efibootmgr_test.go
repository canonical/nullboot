// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"encoding/hex"
	"testing"

	"gopkg.in/check.v1"
)

func decodeHexStringT(t *testing.T, str string) []byte {
	h, err := hex.DecodeString(str)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func decodeHexString(c *check.C, str string) []byte {
	h, err := hex.DecodeString(str)
	c.Assert(err, check.IsNil)
	return h
}

func Test(t *testing.T) { check.TestingT(t) }
