// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"
	"os"
)

const Usage = `usage: nullbootctl <comman> <ptions>

Commands:
  install         Install boot artifacts
  recovery-key    Manage disk encryption recovery keys

See 'nullbootctl <command> -h' for more help.
`

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "recovery-key" {
			os.Args = os.Args[1:] // Strip the first item
			os.Args[0] = "nullbootctl recovery-key"
			cmdRecoveryKey()
			return
		}
		if os.Args[1] == "-h" || os.Args[1] == "--help" {
			fmt.Print(Usage)
			return
		}
		if os.Args[1] == "install" {
			os.Args = os.Args[1:] // Strip this command (it is the default)
			os.Args[0] = "nullbootctl install"
			cmdInstall()
			return
		}
		fmt.Fprintf(os.Stderr, "nullbootctl: '%v' is not a valid command. See 'nullbootctl --help'.\n", os.Args[1])
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "nullbootctl: valid commands: install, recovery-key\n")
	os.Exit(1)
}
