// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

// DefaultConfigPath is the default nullboot configuration file.
const DefaultConfigPath = "/etc/nullboot.conf"

// Configuration describes kernel boot priorities, like
// GRUB_FLAVOUR_ORDER (see debian/grub-sort-version). Each flavour is
// compiled to [\s\S]*-<flavour>(\s*\d*)$ and matched against the
// kernel ABI. Higher priority flavours sort first; unlisted flavours
// have priority 0.
type Configuration struct {
	priorities map[int][]*regexp.Regexp

	KernelPriority map[string]int `yaml:"kernel-priority"`
}

// ReadConfig reads YAML configuration from configPath and any
// drop-ins in configPath.d, e.g.:
//
//	kernel-priority:
//	  fips: 1000
//	  azure-fde: 100
//
// All files are optional; drop-ins merge kernel-priority keys,
// overriding earlier files.
func ReadConfig(configPath string) (*Configuration, error) {
	config := &Configuration{KernelPriority: map[string]int{}}

	paths := []string{configPath}
	entries, err := appFs.ReadDir(configPath + ".d")
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("cannot read configuration drop-in directory: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".conf") {
			paths = append(paths, path.Join(configPath+".d", e.Name()))
		}
	}

	for _, p := range paths {
		if err := config.readFile(p); err != nil {
			return nil, err
		}
	}

	config.priorities = map[int][]*regexp.Regexp{}
	for flavour, priority := range config.KernelPriority {
		re, err := regexp.Compile(`[\s\S]*-` + regexp.QuoteMeta(flavour) + `(\s*\d*)$`)
		if err != nil {
			return nil, fmt.Errorf("invalid flavour %q: %w", flavour, err)
		}
		config.priorities[priority] = append(config.priorities[priority], re)
	}
	return config, nil
}

// readFile merges a single YAML configuration file into c. Missing
// files are ignored; yaml.v2 merges map keys into existing maps.
func (c *Configuration) readFile(configPath string) error {
	file, err := appFs.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("cannot open configuration file %s: %w", configPath, err)
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("cannot read configuration file %s: %w", configPath, err)
	}
	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("cannot parse configuration file %s: %w", configPath, err)
	}
	return nil
}

// weight returns the priority of the flavour matching abi, or 0 if
// none matches.
func (c *Configuration) weight(abi string) int {
	if c == nil {
		return 0
	}
	for priority, res := range c.priorities {
		for _, re := range res {
			if re.MatchString(abi) {
				return priority
			}
		}
	}
	return 0
}
