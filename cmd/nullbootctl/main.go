// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	flags "github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
	"os"
)

// installOptions are the options accepted by the "install" command.
type installOptions struct {
	NoTPM      bool   `long:"no-tpm" description:"Do not do any resealing with the TPM"`
	NoEfivars  bool   `long:"no-efivars" description:"Do not use or update the EFI variables. Disables kernel fallback mechanism"`
	OutputJSON string `long:"output-json" value-name:"<file>" description:"JSON file to write. Disables writing real EFI variables and enablement of the kernel fallback mechanism"`
	NoBootNext bool   `long:"no-boot-next" description:"Disables use of BootNext. This flag must be disabled in order to upgrade to a new kernel version."`
}

// Execute implements [flags.Commander] and is invoked by go-flags when the
// "install" command is given.
func (opts *installOptions) Execute(args []string) error {
	if len(args) != 0 {
		log.Fatalf("nullbootctl: '%v' is not a valid command. See 'nullbootctl --help'.", args[0])
	}
	cmdInstall(opts)
	return nil
}

// Execute implements [flags.Commander] and is invoked by go-flags when the
// "recovery-key" command is given.
func (opts *recoveryKeyOptions) Execute(args []string) error {
	cmdRecoveryKey(opts)
	return nil
}

func main() {
	parser := flags.NewParser(nil, flags.Default)
	parser.SubcommandsOptional = false

	if _, err := parser.AddCommand("install", "Install boot artifacts", "", &installOptions{}); err != nil {
		log.Fatal(err)
	}

	if _, err := parser.AddCommand("recovery-key", "Manage disk encryption recovery keys",
		"Create, list, delete or verify a LUKS2 disk encryption recovery key.", defaultRecoveryKeyOptions); err != nil {
		log.Fatal(err)
	}

	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}
}
