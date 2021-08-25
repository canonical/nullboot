// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efivars

import (
	"testing"
)

func TestVariables_smoke(t *testing.T) {
	if !VariablesSupported() {
		t.Skip("Variables not supported")
	}

	for _, name := range GetVariableNames(GUIDGlobal) {
		GetVariable(GUIDGlobal, name)
		break
	}

}
