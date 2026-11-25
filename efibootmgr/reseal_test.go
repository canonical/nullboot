// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"crypto"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	efi "github.com/canonical/go-efilib"
	"github.com/canonical/go-tpm2"
	"github.com/canonical/go-tpm2/linux"
	"github.com/canonical/tcglog-parser"
	"github.com/snapcore/secboot"
	secboot_tpm2 "github.com/snapcore/secboot/tpm2"

	"golang.org/x/sys/unix"

	"gopkg.in/check.v1"
)

type resealSuite struct {
	mapFsMixin
}

func (*resealSuite) mockSbGetPrimaryKeyFromKernel(fn func(prefix, devicePath string, remove bool) (secboot.PrimaryKey, error)) (restore func()) {
	orig := sbGetPrimaryKeyFromKernel
	sbGetPrimaryKeyFromKernel = fn
	return func() {
		sbGetPrimaryKeyFromKernel = orig
	}
}

func (*resealSuite) mockSbtpmConnectToDefaultTPM(fn func() (*secboot_tpm2.Connection, error)) (restore func()) {
	orig := sbtpmConnectToDefaultTPM
	sbtpmConnectToDefaultTPM = fn
	return func() {
		sbtpmConnectToDefaultTPM = orig
	}
}

func (*resealSuite) mockIsErrKeyDataUnexpectedJson(fn func(err error) bool) (restore func()) {
	orig := isErrKeyDataUnexpectedJson
	isErrKeyDataUnexpectedJson = fn
	return func() {
		isErrKeyDataUnexpectedJson = orig
	}
}

func (*resealSuite) mockNewFileKeyDataReader(fn func(path string) (*secboot.FileKeyDataReader, error)) (restore func()) {
	orig := sbNewFileKeyDataReader
	sbNewFileKeyDataReader = fn
	return func() {
		sbNewFileKeyDataReader = orig
	}
}

func (*resealSuite) mockReadKeyData(fn func(r secboot.KeyDataReader) (*secboot.KeyData, error)) (restore func()) {
	orig := sbReadKeyData
	sbReadKeyData = fn
	return func() {
		sbReadKeyData = orig
	}
}

func (*resealSuite) mockSbtpmNewKeyDataFromSealedKeyObjectFile(fn func(path string) (*secboot.KeyData, error)) (restore func()) {
	orig := sbtpmNewKeyDataFromSealedKeyObjectFile
	sbtpmNewKeyDataFromSealedKeyObjectFile = fn
	return func() {
		sbtpmNewKeyDataFromSealedKeyObjectFile = orig
	}
}

func (*resealSuite) mockSbtpmNewSealedKeyData(fn func(k *secboot.KeyData) (*secboot_tpm2.SealedKeyData, error)) (restore func()) {
	orig := sbtpmNewSealedKeyData
	sbtpmNewSealedKeyData = fn
	return func() {
		sbtpmNewSealedKeyData = orig
	}
}

func (*resealSuite) mockSbtpmSealedKeyDataUpdatePCRProtectionPolicy(fn func(k *secboot_tpm2.SealedKeyData, tpm *secboot_tpm2.Connection, authKey secboot.PrimaryKey, profile *secboot_tpm2.PCRProtectionProfile, policyVersionOption secboot_tpm2.PCRPolicyVersionOption) error) (restore func()) {
	orig := sbtpmSealedKeyDataUpdatePCRProtectionPolicy
	sbtpmSealedKeyDataUpdatePCRProtectionPolicy = fn
	return func() {
		sbtpmSealedKeyDataUpdatePCRProtectionPolicy = orig
	}
}

func (*resealSuite) mockSbKeyDataWriteAtomic(fn func(k *secboot.KeyData, w secboot.KeyDataWriter) error) (restore func()) {
	orig := sbKeyDataWriteAtomic
	sbKeyDataWriteAtomic = fn
	return func() {
		sbKeyDataWriteAtomic = orig
	}
}

func (*resealSuite) mockUnixKeyctlInt(fn func(cmd, arg2, arg3, arg4, arg5 int) (int, error)) (restore func()) {
	orig := unixKeyctlInt
	unixKeyctlInt = fn
	return func() {
		unixKeyctlInt = orig
	}
}

func (*resealSuite) mockEfiArch(arch string) (restore func()) {
	orig := appArchitecture
	appArchitecture = arch
	return func() {
		appArchitecture = orig
	}

}

var _ = check.Suite(&resealSuite{})

func (s *resealSuite) TestTrustedEfiImageOk(c *check.C) {
	s.writeFile(c, "/foo", 0, 43, 50)

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)
	c.Check(assets.TrustNewFromDir("/"), check.IsNil)

	context := new(pcrProfileComputeContext)
	img := newTrustedEFIImage(assets, context, "/foo")

	f, err := img.Open()
	c.Assert(err, check.IsNil)

	c.Check(f.Close(), check.IsNil)
	c.Check(context.nOpen, check.Equals, 0)
	c.Check(context.failedPaths, check.IsNil)
}

func (s *resealSuite) TestTrustedEfiImageBad(c *check.C) {
	s.writeFile(c, "/foo", 0, 43, 50)

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	context := new(pcrProfileComputeContext)
	img := newTrustedEFIImage(assets, context, "/foo")

	f, err := img.Open()
	c.Assert(err, check.IsNil)

	c.Check(f.Close(), check.IsNil)
	c.Check(context.nOpen, check.Equals, 0)
	c.Check(context.failedPaths, check.DeepEquals, []string{"/foo"})
}

type testResealKeyData struct {
	arch          string
	primaryKey    secboot.PrimaryKey
	devicePaths   []string
	shims         [][]byte
	kernels       [][]byte
	sealedKeyJson bool // set it to true to test with sealed-key in JSON format
}

func (s *resealSuite) testResealKey(c *check.C, data *testResealKeyData) {
	var (
		expectedKeyData                     *secboot.KeyData            = nil
		expectedNewFileKeyDataReader        *secboot.FileKeyDataReader  = nil
		expectedSkd                         *secboot_tpm2.SealedKeyData = nil
		expectedTpm                         *secboot_tpm2.Connection    = nil
		userKeyringLinkedFromProcessKeyring                             = false
	)

	restore := s.mockEfiArch(data.arch)
	defer restore()

	introspectLoadChains = func(
		pcrAlg tpm2.HashAlgorithmId,
		rootBranch *secboot_tpm2.PCRProtectionProfileBranch,
		loadChains []*LoadChain) {
		c.Assert(rootBranch, check.NotNil)
		c.Check(pcrAlg, check.Equals, tpm2.HashAlgorithmSHA256)

		c.Assert(loadChains, check.HasLen, len(data.shims))
		for i, e := range loadChains {
			f, err := e.Open()
			c.Assert(err, check.IsNil)

			r := io.NewSectionReader(f, 0, 1<<63-1)
			b, err := ioutil.ReadAll(r)
			c.Check(err, check.IsNil)
			f.Close()

			c.Check(b, check.DeepEquals, data.shims[i])

			c.Assert(e.Next, check.HasLen, len(data.kernels))
			for i, e := range e.Next {
				f, err := e.Open()
				c.Assert(err, check.IsNil)

				r := io.NewSectionReader(f, 0, 1<<63-1)
				b, err := ioutil.ReadAll(r)
				c.Check(err, check.IsNil)
				f.Close()

				c.Check(b, check.DeepEquals, data.kernels[i])
			}
		}

		rootBranch.AddPCRValue(tpm2.HashAlgorithmSHA256, 4, make([]byte, 32))

		c.Assert(loadChains, check.HasLen, len(data.shims))
		for i, e := range loadChains {
			f, err := e.Open()
			c.Assert(err, check.IsNil)

			r := io.NewSectionReader(f, 0, 1<<63-1)
			b, err := ioutil.ReadAll(r)
			c.Check(err, check.IsNil)
			f.Close()

			c.Check(b, check.DeepEquals, data.shims[i])

			c.Assert(e.Next, check.HasLen, len(data.kernels))
			for i, e := range e.Next {
				f, err := e.Open()
				c.Assert(err, check.IsNil)

				r := io.NewSectionReader(f, 0, 1<<63-1)
				b, err := ioutil.ReadAll(r)
				c.Check(err, check.IsNil)
				f.Close()

				c.Check(b, check.DeepEquals, data.kernels[i])
			}
		}

		rootBranch.AddPCRValue(tpm2.HashAlgorithmSHA256, 7, make([]byte, 32))
	}
	defer func() { introspectLoadChains = nil }()

	n := 0
	restore = s.mockSbGetPrimaryKeyFromKernel(func(prefix, devicePath string, remove bool) (secboot.PrimaryKey, error) {
		c.Check(prefix, check.Equals, "ubuntu-fde")
		c.Check(devicePath, check.Equals, data.devicePaths[n])
		c.Check(remove, check.Equals, false)

		c.Check(userKeyringLinkedFromProcessKeyring, check.Equals, true)

		n++
		if n < len(data.devicePaths) {
			return nil, secboot.ErrKernelKeyNotFound
		}

		return data.primaryKey, nil
	})
	defer restore()

	restore = s.mockSbtpmConnectToDefaultTPM(func() (*secboot_tpm2.Connection, error) {
		c.Check(expectedTpm, check.IsNil)

		tcti, err := linux.OpenDevice("/dev/null")
		c.Assert(err, check.IsNil)

		expectedTpm = &secboot_tpm2.Connection{TPMContext: tpm2.NewTPMContext(tcti)}
		return expectedTpm, nil
	})
	defer restore()

	if data.sealedKeyJson {
		// Trigger the alternate code path when the sealed key is in JSON format
		restore = s.mockIsErrKeyDataUnexpectedJson(func(error) bool { return true })
		defer restore()
		restore = s.mockSbtpmNewKeyDataFromSealedKeyObjectFile(func(path string) (*secboot.KeyData, error) {
			return nil, fmt.Errorf("mocked error")
		})
		defer restore()
		restore = s.mockNewFileKeyDataReader(func(path string) (*secboot.FileKeyDataReader, error) {
			c.Check(expectedNewFileKeyDataReader, check.IsNil)
			c.Check(path, check.Equals, "/boot/efi/device/fde/cloudimg-rootfs.sealed-key")
			expectedNewFileKeyDataReader = &secboot.FileKeyDataReader{}
			return expectedNewFileKeyDataReader, nil
		})
		defer restore()
		restore = s.mockReadKeyData(func(r secboot.KeyDataReader) (*secboot.KeyData, error) {
			c.Check(r, check.Equals, expectedNewFileKeyDataReader)
			c.Check(expectedKeyData, check.IsNil)
			expectedKeyData = &secboot.KeyData{}
			return expectedKeyData, nil
		})
		defer restore()

	} else {
		restore = s.mockSbtpmNewKeyDataFromSealedKeyObjectFile(func(path string) (*secboot.KeyData, error) {
			c.Check(expectedKeyData, check.IsNil)
			c.Check(path, check.Equals, "/boot/efi/device/fde/cloudimg-rootfs.sealed-key")
			expectedKeyData = &secboot.KeyData{}
			return expectedKeyData, nil
		})
		defer restore()
	}

	restore = s.mockSbtpmNewSealedKeyData(func(k *secboot.KeyData) (*secboot_tpm2.SealedKeyData, error) {
		c.Check(k, check.Equals, expectedKeyData)
		c.Check(expectedSkd, check.IsNil)

		expectedSkd = &secboot_tpm2.SealedKeyData{}
		return expectedSkd, nil
	})
	defer restore()

	restore = s.mockSbtpmSealedKeyDataUpdatePCRProtectionPolicy(func(k *secboot_tpm2.SealedKeyData, tpm *secboot_tpm2.Connection, authKey secboot.PrimaryKey, profile *secboot_tpm2.PCRProtectionProfile, policyVersionOption secboot_tpm2.PCRPolicyVersionOption) error {
		c.Check(k, check.Equals, expectedSkd)
		c.Check(tpm, check.Equals, expectedTpm)
		c.Check(authKey, check.DeepEquals, data.primaryKey)
		c.Check(policyVersionOption, check.Equals, secboot_tpm2.NewPCRPolicyVersion)
		c.Assert(profile, check.NotNil)

		pcrs, _, err := profile.ComputePCRDigests(nil, tpm2.HashAlgorithmSHA256)
		c.Check(err, check.IsNil)
		// NOTE: does not seem like the PCR selection list thingy has a better way to introspect itself...
		c.Check(pcrs.String(), check.Equals, "[{hash:TPM_ALG_SHA256, select:[4 7 12]}]")
		return nil
	})
	defer restore()

	restore = s.mockSbKeyDataWriteAtomic(func(k *secboot.KeyData, w secboot.KeyDataWriter) error {
		c.Check(k, check.Equals, expectedKeyData)
		fw, ok := w.(*secboot.FileKeyDataWriter)
		c.Check(ok, check.Equals, true)
		c.Check(fw, check.NotNil)
		return nil
	})
	defer restore()

	restore = s.mockUnixKeyctlInt(func(cmd, arg2, arg3, arg4, arg5 int) (int, error) {
		if cmd == unix.KEYCTL_LINK && arg2 == -4 && arg3 == -2 && arg4 == 0 && arg5 == 0 {
			userKeyringLinkedFromProcessKeyring = true
		}
		return 0, nil
	})
	defer restore()

	mockvars := MockEFIVariables{map[efi.VariableDescriptor]mockEFIVariable{{GUID: efi.GlobalVariable, Name: "BootOrder"}: {[]byte{1, 0, 2, 0, 3, 0}, 123}}}

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	c.Check(assets.TrustNewFromDir("/boot/efi/EFI/ubuntu"), check.IsNil)

	c.Check(assets.TrustNewFromDir("/usr/lib/nullboot/shim"), check.IsNil)
	c.Check(assets.TrustNewFromDir("/usr/lib/linux"), check.IsNil)

	bm, err := NewBootManagerForVariables(&mockvars)
	c.Assert(err, check.IsNil)
	km, err := NewKernelManager("/boot/efi", "/usr/lib/linux", "ubuntu", &bm)
	c.Assert(err, check.IsNil)

	c.Check(ResealKey(assets, km, "/boot/efi", "/usr/lib/nullboot/shim", "ubuntu"), check.IsNil)
}

func (s *resealSuite) TestResealKeyNoFDE(c *check.C) {
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel-efi.1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{})
}

func (s *resealSuite) TestResealKeyBeforeNewKernel(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:        "x64",
		primaryKey:  []byte{1, 2, 3, 4, 5, 6},
		devicePaths: []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim1"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel2"),
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyAfterNewKernel(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:        "x64",
		primaryKey:  []byte{1, 2, 3, 4, 5, 6},
		devicePaths: []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim1"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel2"),
			[]byte("kernel1"),
			[]byte("kernel2"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyBeforeNewShim(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:        "x64",
		primaryKey:  []byte{1, 2, 3, 4, 5, 6},
		devicePaths: []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim2"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyAfterNewShim(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:        "x64",
		primaryKey:  []byte{1, 2, 3, 4, 5, 6},
		devicePaths: []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim2"),
			[]byte("shim2"),
		},
		kernels: [][]byte{
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyBeforeRemoveKernel(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:        "x64",
		primaryKey:  []byte{1, 2, 3, 4, 5, 6},
		devicePaths: []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim1"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel2"),
			[]byte("kernel2"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyAfterRemoveKernel(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:        "x64",
		primaryKey:  []byte{1, 2, 3, 4, 5, 6},
		devicePaths: []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim1"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel2"),
			[]byte("kernel2"),
		},
	})
}

func (s *resealSuite) TestResealKeyDifferentPrimaryKey(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:        "x64",
		primaryKey:  []byte{5, 6, 7, 8, 9},
		devicePaths: []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim2"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyDifferentBlockDevice(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/vda14", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/vda14", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:        "x64",
		primaryKey:  []byte{1, 2, 3, 4, 5, 6},
		devicePaths: []string{"/dev/vda14"},
		shims: [][]byte{
			[]byte("shim2"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyDifferentArch(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimaa64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimaa64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:        "aa64",
		primaryKey:  []byte{1, 2, 3, 4, 5, 6},
		devicePaths: []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim1"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel2"),
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyGetPrimaryKeyFromKernelBug(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	c.Check(s.fs.WriteFile("/dev/sda15", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")
	s.symlink(c, "/dev/sda1", "/dev/disk/by-partuuid/94725587-885d-4bde-bc61-078e0010057d")
	s.symlink(c, "/dev/sda15", "/dev/disk/by-partuuid/848b8304-0f20-42e9-9806-b447ce344d85")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:        "x64",
		primaryKey:  []byte{1, 2, 3, 4, 5, 6},
		devicePaths: []string{"/dev/sda1", "/dev/disk/by-partuuid/94725587-885d-4bde-bc61-078e0010057d"},
		shims: [][]byte{
			[]byte("shim2"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyJson(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:        "x64",
		primaryKey:  []byte{1, 2, 3, 4, 5, 6},
		devicePaths: []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim1"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
		sealedKeyJson: true,
	})
}

type testResealKeyUnhappyData struct {
	noAuxKey        bool
	fileLeak        bool
	untrustedAssets bool
	noTpm           bool
}

func (s *resealSuite) testResealKeyUnhappy(c *check.C, data *testResealKeyUnhappyData) error {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	restore := s.mockEfiArch("x64")
	defer restore()

	introspectLoadChains = func(
		pcrAlg tpm2.HashAlgorithmId,
		rootBranch *secboot_tpm2.PCRProtectionProfileBranch,
		loadChains []*LoadChain) {

		if data.fileLeak {
			loadChains[0].Open()
		}
		for _, e := range loadChains {
			f, err := e.Open()
			c.Assert(err, check.IsNil)
			f.Close()

			for _, e := range e.Next {
				f, err := e.Open()
				c.Assert(err, check.IsNil)
				f.Close()
			}
		}
		for _, e := range loadChains {
			f, err := e.Open()
			c.Assert(err, check.IsNil)
			f.Close()

			for _, e := range e.Next {
				f, err := e.Open()
				c.Assert(err, check.IsNil)
				f.Close()
			}
		}
	}
	defer func() { introspectLoadChains = nil }()

	restore = s.mockSbGetPrimaryKeyFromKernel(func(prefix, devicePath string, remove bool) (secboot.PrimaryKey, error) {
		if data.noAuxKey {
			return nil, secboot.ErrKernelKeyNotFound
		}
		return nil, nil
	})
	defer restore()

	restore = s.mockSbtpmConnectToDefaultTPM(func() (*secboot_tpm2.Connection, error) {
		if data.noTpm {
			return nil, secboot_tpm2.ErrNoTPM2Device
		}
		tcti, err := linux.OpenDevice("/dev/null")
		c.Assert(err, check.IsNil)

		return &secboot_tpm2.Connection{TPMContext: tpm2.NewTPMContext(tcti)}, nil
	})
	defer restore()

	restore = s.mockSbtpmNewKeyDataFromSealedKeyObjectFile(func(path string) (*secboot.KeyData, error) {
		return &secboot.KeyData{}, nil
	})
	defer restore()

	restore = s.mockSbtpmNewSealedKeyData(func(k *secboot.KeyData) (*secboot_tpm2.SealedKeyData, error) {
		return &secboot_tpm2.SealedKeyData{}, nil
	})
	defer restore()

	restore = s.mockSbtpmSealedKeyDataUpdatePCRProtectionPolicy(func(k *secboot_tpm2.SealedKeyData, tpm *secboot_tpm2.Connection, authKey secboot.PrimaryKey, profile *secboot_tpm2.PCRProtectionProfile, policyVersionOption secboot_tpm2.PCRPolicyVersionOption) error {
		return nil
	})
	defer restore()

	restore = s.mockSbKeyDataWriteAtomic(func(k *secboot.KeyData, w secboot.KeyDataWriter) error {
		return nil
	})
	defer restore()

	restore = s.mockUnixKeyctlInt(func(cmd, arg2, arg3, arg4, arg5 int) (int, error) {
		return 0, nil
	})
	defer restore()

	mockvars := MockEFIVariables{map[efi.VariableDescriptor]mockEFIVariable{{GUID: efi.GlobalVariable, Name: "BootOrder"}: {[]byte{1, 0, 2, 0, 3, 0}, 123}}}

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	if !data.untrustedAssets {
		c.Check(assets.TrustNewFromDir("/boot/efi/EFI/ubuntu"), check.IsNil)
	}
	c.Check(assets.TrustNewFromDir("/usr/lib/nullboot/shim"), check.IsNil)
	c.Check(assets.TrustNewFromDir("/usr/lib/linux"), check.IsNil)

	bm, err := NewBootManagerForVariables(&mockvars)
	c.Assert(err, check.IsNil)
	km, err := NewKernelManager("/boot/efi", "/usr/lib/linux", "ubuntu", &bm)
	c.Assert(err, check.IsNil)

	return ResealKey(assets, km, "/boot/efi", "/usr/lib/nullboot/shim", "ubuntu")
}

func (s *resealSuite) TestResealKeyUnhappyNoPrimaryKey(c *check.C) {
	err := s.testResealKeyUnhappy(c, &testResealKeyUnhappyData{
		noAuxKey: true,
	})
	c.Check(err, check.ErrorMatches, "cannot obtain auth key from kernel: cannot read key from kernel: cannot find key in kernel keyring")
}

func (s *resealSuite) TestResealKeyUnhappyFileLeak(c *check.C) {
	err := s.testResealKeyUnhappy(c, &testResealKeyUnhappyData{
		fileLeak: true,
	})
	c.Check(err, check.ErrorMatches, "leaked open files from computing PCR profile")
}

func (s *resealSuite) TestResealKeyUnhappyUntrustedAssets(c *check.C) {
	err := s.testResealKeyUnhappy(c, &testResealKeyUnhappyData{
		untrustedAssets: true,
	})
	c.Check(err, check.ErrorMatches, "some assets failed an integrity check: \\[/boot/efi/EFI/ubuntu/shimx64.efi /boot/efi/EFI/ubuntu/shimx64.efi\\]")
}

func (s *resealSuite) TestResealKeyUnhappyNoTPM(c *check.C) {
	err := s.testResealKeyUnhappy(c, &testResealKeyUnhappyData{
		noTpm: true,
	})
	c.Check(err, check.ErrorMatches, "no TPM2 device is available")
}

// The TCG log writing code is borrowed from github.com:canonical/secboot to avoid checking in a binary log

type logHashData interface {
	Write(w io.Writer) error
}

type bytesHashData []byte

func (d bytesHashData) Write(w io.Writer) error {
	_, err := w.Write(d)
	return err
}

type logEvent struct {
	pcrIndex  tpm2.Handle
	eventType tcglog.EventType
	data      tcglog.EventData
}

type logBuilder struct {
	algs   []tpm2.HashAlgorithmId
	events []*tcglog.Event
}

func (b *logBuilder) hashLogExtendEvent(c *check.C, data logHashData, event *logEvent) {
	ev := &tcglog.Event{
		PCRIndex:  event.pcrIndex,
		EventType: event.eventType,
		Digests:   make(tcglog.DigestMap),
		Data:      event.data}

	for _, alg := range b.algs {
		h := alg.NewHash()
		c.Assert(data.Write(h), check.IsNil)
		ev.Digests[alg] = h.Sum(nil)
	}

	b.events = append(b.events, ev)
}

func (s *resealSuite) writeMockTcglog(c *check.C) {
	builder := &logBuilder{algs: []tpm2.HashAlgorithmId{tpm2.HashAlgorithmSHA1, tpm2.HashAlgorithmSHA256}}

	var digestSizes []tcglog.EFISpecIdEventAlgorithmSize
	for _, alg := range builder.algs {
		digestSizes = append(digestSizes,
			tcglog.EFISpecIdEventAlgorithmSize{
				AlgorithmId: alg,
				DigestSize:  uint16(alg.Size()),
			})
	}

	builder.events = []*tcglog.Event{
		{
			PCRIndex:  0,
			EventType: tcglog.EventTypeNoAction,
			Digests:   tcglog.DigestMap{tpm2.HashAlgorithmSHA1: make(tpm2.Digest, tpm2.HashAlgorithmSHA1.Size())},
			Data: &tcglog.SpecIdEvent03{
				SpecVersionMajor: 2,
				UintnSize:        2,
				DigestSizes:      digestSizes,
			},
		},
	}

	{
		data := &tcglog.SeparatorEventData{Value: tcglog.SeparatorEventNormalValue}
		builder.hashLogExtendEvent(c, data, &logEvent{
			pcrIndex:  7,
			eventType: tcglog.EventTypeSeparator,
			data:      data})
	}
	{
		data := tcglog.EFICallingEFIApplicationEvent
		builder.hashLogExtendEvent(c, data, &logEvent{
			pcrIndex:  4,
			eventType: tcglog.EventTypeEFIAction,
			data:      data})
	}
	for _, pcr := range []tpm2.Handle{0, 1, 2, 3, 4, 5, 6} {
		data := &tcglog.SeparatorEventData{Value: tcglog.SeparatorEventNormalValue}
		builder.hashLogExtendEvent(c, data, &logEvent{
			pcrIndex:  pcr,
			eventType: tcglog.EventTypeSeparator,
			data:      data})
	}
	{
		pe := bytesHashData("mock shim PE")
		data := &tcglog.EFIImageLoadEvent{
			LocationInMemory: 0x6556c018,
			LengthInMemory:   955072,
			DevicePath: efi.DevicePath{
				&efi.ACPIDevicePathNode{
					HID: 0x0a0341d0,
					UID: 0x0},
				&efi.PCIDevicePathNode{
					Function: 0x0,
					Device:   0x1d},
				&efi.PCIDevicePathNode{
					Function: 0x0,
					Device:   0x0},
				&efi.NVMENamespaceDevicePathNode{
					NamespaceID:   0x1,
					NamespaceUUID: efi.EUI64{}},
				&efi.HardDriveDevicePathNode{
					PartitionNumber: 1,
					PartitionStart:  0x800,
					PartitionSize:   0x100000,
					Signature:       efi.GUIDHardDriveSignature(efi.MakeGUID(0x66de947b, 0xfdb2, 0x4525, 0xb752, [...]uint8{0x30, 0xd6, 0x6b, 0xb2, 0xb9, 0x60})),
					MBRType:         efi.GPT},
				efi.FilePathDevicePathNode("\\EFI\\ubuntu\\shimx64.efi")}}
		builder.hashLogExtendEvent(c, pe, &logEvent{
			pcrIndex:  4,
			eventType: tcglog.EventTypeEFIBootServicesApplication,
			data:      data})
	}
	{
		pe := bytesHashData("mock kernel PE")
		data := &tcglog.EFIImageLoadEvent{
			DevicePath: efi.DevicePath{efi.FilePathDevicePathNode("\\EFI\\ubuntu\\kernel.efi-1.0-1-generic")}}
		builder.hashLogExtendEvent(c, pe, &logEvent{
			pcrIndex:  4,
			eventType: tcglog.EventTypeEFIBootServicesApplication,
			data:      data})
	}

	f, err := s.fs.OpenFile("/sys/kernel/security/tpm0/binary_bios_measurements", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	c.Assert(err, check.IsNil)
	defer f.Close()

	c.Check(tcglog.NewLogForTesting(builder.events).Write(f), check.IsNil)
}

func (s *resealSuite) mockEfiComputePeImageDigest(fn func(alg crypto.Hash, r io.ReaderAt, sz int64) ([]byte, error)) (restore func()) {
	orig := efiComputePeImageDigest
	efiComputePeImageDigest = fn

	return func() {
		efiComputePeImageDigest = orig
	}
}

func (s *resealSuite) TestTrustCurrentBoot(c *check.C) {
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.writeMockTcglog(c)

	restore := s.mockEfiComputePeImageDigest(func(alg crypto.Hash, r io.ReaderAt, sz int64) ([]byte, error) {
		r2 := io.NewSectionReader(r, 0, sz)
		b, err := ioutil.ReadAll(r2)
		c.Check(err, check.IsNil)

		switch {
		case bytes.Equal(b, []byte("shim1")):
			return decodeHexString(c, "93c294bd9d372cf76e3cfd6f66a93fd2586aeb0406677ea0df104349b2ec093d"), nil
		case bytes.Equal(b, []byte("kernel1")):
			return decodeHexString(c, "54a5737f95928a359ba326bda6405a8e91fd06869cdb76f7f53aae83c1050308"), nil
		default:
			c.Fatal("invalid file")
		}
		return nil, nil
	})
	defer restore()

	assets := newTrustedAssets()

	c.Check(TrustCurrentBoot(assets, "/boot/efi"), check.IsNil)

	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte{
		decodeHexString(c, "efbef08d5d3787d609ec6b55fabc36c7f212140b97a88606a39dc8f732368147"),
		decodeHexString(c, "7e8c4310bd1e228888917fb5f87920426dbecd64ea7d6c2256740f80e39dcf6f")})
	c.Check(assets.newAssets, check.DeepEquals, [][]byte{
		decodeHexString(c, "efbef08d5d3787d609ec6b55fabc36c7f212140b97a88606a39dc8f732368147"),
		decodeHexString(c, "7e8c4310bd1e228888917fb5f87920426dbecd64ea7d6c2256740f80e39dcf6f")})
}

func (s *resealSuite) TestTrustCurrentBootRejectPeHashMismatch(c *check.C) {
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.writeMockTcglog(c)

	restore := s.mockEfiComputePeImageDigest(func(alg crypto.Hash, r io.ReaderAt, sz int64) ([]byte, error) {
		r2 := io.NewSectionReader(r, 0, sz)
		b, err := ioutil.ReadAll(r2)
		c.Check(err, check.IsNil)

		switch {
		case bytes.Equal(b, []byte("shim1")):
			return decodeHexString(c, "93c294bd9d372cf76e3cfd6f66a93fd2586aeb0406677ea0df104349b2ec093f"), nil
		case bytes.Equal(b, []byte("kernel1")):
			return decodeHexString(c, "54a5737f95928a359ba326bda6405a8e91fd06869cdb76f7f53aae83c1050308"), nil
		default:
			c.Fatal("invalid file")
		}
		return nil, nil
	})
	defer restore()

	assets := newTrustedAssets()

	c.Check(TrustCurrentBoot(assets, "/boot/efi"), check.IsNil)

	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte{
		decodeHexString(c, "7e8c4310bd1e228888917fb5f87920426dbecd64ea7d6c2256740f80e39dcf6f")})
	c.Check(assets.newAssets, check.DeepEquals, [][]byte{
		decodeHexString(c, "7e8c4310bd1e228888917fb5f87920426dbecd64ea7d6c2256740f80e39dcf6f")})
}

func (s *resealSuite) TestTrustCurrentBootRejectMissing(c *check.C) {
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.writeMockTcglog(c)

	restore := s.mockEfiComputePeImageDigest(func(alg crypto.Hash, r io.ReaderAt, sz int64) ([]byte, error) {
		r2 := io.NewSectionReader(r, 0, sz)
		b, err := ioutil.ReadAll(r2)
		c.Check(err, check.IsNil)

		switch {
		case bytes.Equal(b, []byte("shim1")):
			return decodeHexString(c, "93c294bd9d372cf76e3cfd6f66a93fd2586aeb0406677ea0df104349b2ec093d"), nil
		case bytes.Equal(b, []byte("kernel1")):
			return decodeHexString(c, "54a5737f95928a359ba326bda6405a8e91fd06869cdb76f7f53aae83c1050308"), nil
		default:
			c.Fatal("invalid file")
		}
		return nil, nil
	})
	defer restore()

	assets := newTrustedAssets()

	c.Check(TrustCurrentBoot(assets, "/boot/efi"), check.IsNil)

	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte{
		decodeHexString(c, "efbef08d5d3787d609ec6b55fabc36c7f212140b97a88606a39dc8f732368147")})
	c.Check(assets.newAssets, check.DeepEquals, [][]byte{
		decodeHexString(c, "efbef08d5d3787d609ec6b55fabc36c7f212140b97a88606a39dc8f732368147")})
}
