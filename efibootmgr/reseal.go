// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"crypto"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	efi "github.com/canonical/go-efilib"
	"github.com/canonical/go-tpm2"
	"github.com/canonical/tcglog-parser"
	"github.com/snapcore/secboot"
	secboot_efi "github.com/snapcore/secboot/efi"
	secboot_tpm2 "github.com/snapcore/secboot/tpm2"

	"golang.org/x/sys/unix"
)

const (
	keyFilePath   = "device/fde/cloudimg-rootfs.sealed-key"
	keyringPrefix = "ubuntu-fde"
	RootfsLabel   = "cloudimg-rootfs-enc"
)

var (
	efiComputePeImageDigest                       = efi.ComputePeImageDigest
	sbGetPrimaryKeyFromKernel                     = secboot.GetPrimaryKeyFromKernel
	sbtpmConnectToDefaultTPM                      = secboot_tpm2.ConnectToDefaultTPM
	sbtpmReadSealedKeyObjectFromFile              = secboot_tpm2.ReadSealedKeyObjectFromFile
	sbtpmSealedKeyObjectUpdatePCRProtectionPolicy = (*secboot_tpm2.SealedKeyObject).UpdatePCRProtectionPolicy
	sbtpmSealedKeyObjectWriteAtomic               = (*secboot_tpm2.SealedKeyObject).WriteAtomic

	unixKeyctlInt = unix.KeyctlInt
)

type pcrProfileComputeContext struct {
	nOpen       int
	failedPaths []string
}

// trustedEFIImage is an implementation of secboot_efi.Image that makes
// use of hashedFile in order to ensure that boot assets added to a PCR
// profile are trusted.
type trustedEFIImage struct {
	assets  *TrustedAssets
	context *pcrProfileComputeContext
	path    string
}

func (i *trustedEFIImage) String() string {
	return i.path
}

func (i *trustedEFIImage) Open() (imageReader secboot_efi.ImageReader, err error) {
	f, err := appFs.Open(i.path)
	if err != nil {
		return nil, err
	}
	i.context.nOpen++

	defer func() {
		if err != nil {
			f.Close()
			i.context.nOpen--
		}
	}()

	return newCheckedHashedFile(f, i.assets, func(trusted bool) {
		if !trusted {
			i.context.failedPaths = append(i.context.failedPaths, i.path)
		}
		i.context.nOpen--
	})
}

func newTrustedEFIImage(assets *TrustedAssets, context *pcrProfileComputeContext, path string) *trustedEFIImage {
	return &trustedEFIImage{assets, context, path}
}

func resolveLink(path string) (string, error) {
	path = filepath.Clean(path)

	for {
		tgtPath, err := appFs.Readlink(path)

		if errors.Is(err, syscall.EINVAL) {
			return path, nil
		}

		if err != nil {
			return "", err
		}

		if !filepath.IsAbs(tgtPath) {
			tgtPath = filepath.Clean(filepath.Join(filepath.Dir(path), tgtPath))
		}

		path = tgtPath
	}
}

func getPrimaryKeyFromKernel() (secboot.PrimaryKey, error) {
	devPath, err := resolveLink(filepath.Join("/dev/disk/by-label", RootfsLabel))
	if err != nil {
		return nil, fmt.Errorf("cannot resolve devive symlink: %w", err)
	}

	// By default, system services get their own session keyring that doesn't have
	// the user keyring linked to it. This means that attempting to read a key from
	// the user keyring will fail if the key only permits possessor read. Link the
	// user keyring into our process keyring so that we can read such keys from the
	// user keyring.
	if _, err := unixKeyctlInt(unix.KEYCTL_LINK, -4, -2, 0, 0); err != nil {
		return nil, fmt.Errorf("cannot link user keyring into process keyring: %w", err)
	}

	key, err := sbGetPrimaryKeyFromKernel(keyringPrefix, devPath, false)
	if err != nil {
		if err == secboot.ErrKernelKeyNotFound {
			// Work around a secboot bug
			ents, err2 := appFs.ReadDir("/dev/disk/by-partuuid")
			if err2 == nil {
				for _, ent := range ents {
					path := filepath.Join("/dev/disk/by-partuuid", ent.Name())
					devPath2, err2 := resolveLink(path)
					if err2 != nil {
						continue
					}

					if devPath2 == devPath {
						key, err = sbGetPrimaryKeyFromKernel(keyringPrefix, path, false)
						break
					}
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read key from kernel: %w", err)
		}
	}

	return key, nil
}

// This LoadChain stuff is copied from snapd
// It gives us a structure we can introspect in unit tests as oppossed
// to the new secboot structures which are now fully opaque.

type LoadChain struct {
	*trustedEFIImage
	// Next is a list of alternative chains that can be loaded
	// following the boot file.
	Next []*LoadChain
}

func NewLoadChain(image *trustedEFIImage, next ...*LoadChain) *LoadChain {
	return &LoadChain{
		trustedEFIImage: image,
		Next:            next,
	}
}

func buildLoadSequences(chains []*LoadChain) (loadseqs *secboot_efi.ImageLoadSequences, err error) {
	// this will build load event trees for the current
	// device configuration, e.g. something like:
	//
	// shim -> kernel 1
	//     |-> kernel 2
	//     |-> kernel ...

	loadseqs = secboot_efi.NewImageLoadSequences()

	for _, chain := range chains {
		// root of load events has source Firmware
		loadseq, err := chain.loadEvent()
		if err != nil {
			return nil, err
		}
		loadseqs.Append(loadseq)
	}
	return loadseqs, nil
}

// loadEvent builds the corresponding load event and its tree
func (lc *LoadChain) loadEvent() (secboot_efi.ImageLoadActivity, error) {
	var next []secboot_efi.ImageLoadActivity
	for _, nextChain := range lc.Next {
		// everything that is not the root has source shim
		ev, err := nextChain.loadEvent()
		if err != nil {
			return nil, err
		}
		next = append(next, ev)
	}
	return secboot_efi.NewImageLoadActivity(lc).Loads(next...), nil
}

// Hook for unit tests to introspect load chains
var introspectLoadChains func(
	pcrAlg tpm2.HashAlgorithmId,
	rootBranch *secboot_tpm2.PCRProtectionProfileBranch,
	loadChains []*LoadChain) = nil

func computePCRProtectionProfile(loadChains []*LoadChain) (*secboot_tpm2.PCRProtectionProfile, error) {
	profile := secboot_tpm2.NewPCRProtectionProfile()

	var options []secboot_efi.PCRProfileOption
	options = append(options,
		secboot_efi.WithSecureBootPolicyProfile(),
		secboot_efi.WithBootManagerCodeProfile(),
	)

	if introspectLoadChains != nil {
		introspectLoadChains(tpm2.HashAlgorithmSHA256, profile.RootBranch(), loadChains)
	} else {
		loadSeqs, err := buildLoadSequences(loadChains)
		if err != nil {
			return nil, err
		}

		if err := secboot_efi.AddPCRProfile(
			tpm2.HashAlgorithmSHA256,
			profile.RootBranch(),
			loadSeqs,
			options...,
		); err != nil {
			return nil, fmt.Errorf("cannot add PCR profile: %w", err)
		}
	}

	profile.RootBranch().AddPCRValue(tpm2.HashAlgorithmSHA256, 12, make([]byte, tpm2.HashAlgorithmSHA256.Size()))

	// snap-bootstrap measures an epoch
	h := crypto.SHA256.New()
	binary.Write(h, binary.LittleEndian, uint32(0))
	profile.RootBranch().ExtendPCR(tpm2.HashAlgorithmSHA256, 12, h.Sum(nil))

	// XXX: The kernel EFI stub has a compiled-in commandline which isn't measured.

	log.Println("Computed PCR profile:", profile)
	pcrValues, err := profile.ComputePCRValues(nil)
	if err != nil {
		return nil, fmt.Errorf("cannot compute PCR values: %w", err)
	}
	log.Println("Computed PCR values:")
	for i, values := range pcrValues {
		log.Printf(" branch %d:\n", i)
		for alg := range values {
			for pcr := range values[alg] {
				log.Printf("  PCR%d,%v: %x\n", pcr, alg, values[alg][pcr])
			}
		}
	}
	pcrs, digests, err := profile.ComputePCRDigests(nil, tpm2.HashAlgorithmSHA256)
	if err != nil {
		return nil, fmt.Errorf("cannot compute PCR digests: %w", err)
	}
	log.Println("PCR selection:", pcrs)
	log.Println("Computed PCR digests:")
	for _, digest := range digests {
		log.Printf(" %x\n", digest)
	}

	return profile, nil
}

// ResealKey updates the PCR profile for the disk encryption key to incorporate
// the boot assets installed directly by the package manager and those assets
// copied by this package to the ESP.
func ResealKey(assets *TrustedAssets, km *KernelManager, esp, shimSource, vendor string) error {
	_, err := appFs.Stat(filepath.Join(esp, keyFilePath))
	if os.IsNotExist(err) {
		// Assume that this file being missing means there is nothing to do.
		return nil
	}

	context := new(pcrProfileComputeContext)

	var kernels []*LoadChain

	sourceKernelNames := []string{}
	for _, sk := range km.sourceKernels {
		sourceKernelNames = append(sourceKernelNames, sk.GetKernelName())
	}
	targetKernelNames := []string{}
	for _, tk := range km.targetKernels {
		targetKernelNames = append(targetKernelNames, tk.GetKernelName())
	}

	for _, x := range []struct {
		dir   string
		files []string
	}{
		{
			dir:   km.sourceDir,
			files: sourceKernelNames,
		},
		{
			dir:   km.targetDir,
			files: targetKernelNames,
		},
	} {
		for _, n := range x.files {
			path := filepath.Join(x.dir, n)

			kernels = append(kernels, NewLoadChain(newTrustedEFIImage(assets, context, path)))
		}
	}

	shimBase := "shim" + GetEfiArchitecture() + ".efi"

	var shims []*LoadChain

	for _, path := range []string{
		filepath.Join(shimSource, shimBase+".signed"),
		filepath.Join(esp, "EFI", vendor, shimBase)} {
		_, err := appFs.Stat(path)
		if os.IsNotExist(err) {
			continue
		}

		shims = append(shims, NewLoadChain(newTrustedEFIImage(assets, context, path), kernels...))
	}

	authKey, err := getPrimaryKeyFromKernel()
	if err != nil {
		return fmt.Errorf("cannot obtain auth key from kernel: %w", err)
	}

	pcrProfile, err := computePCRProtectionProfile(shims)
	if err != nil {
		return fmt.Errorf("cannot compute PCR profile: %w", err)
	}

	if context.nOpen != 0 {
		return errors.New("leaked open files from computing PCR profile")
	}

	if len(context.failedPaths) > 0 {
		return fmt.Errorf("some assets failed an integrity check: %v", context.failedPaths)
	}

	k, err := sbtpmReadSealedKeyObjectFromFile(filepath.Join(esp, keyFilePath))
	if err != nil {
		return fmt.Errorf("cannot read sealed key file: %w", err)
	}

	// XXX: Connection is required because we do integrity checks
	// on the key data. Should probably switch to using the /dev/tpmrm0
	// device here.
	tpm, err := sbtpmConnectToDefaultTPM()
	if err != nil {
		return err
	}
	defer tpm.Close()

	if err := sbtpmSealedKeyObjectUpdatePCRProtectionPolicy(k, tpm, authKey, pcrProfile); err != nil {
		return fmt.Errorf("cannot update PCR profile: %w", err)
	}

	w := secboot_tpm2.NewFileSealedKeyObjectWriter(filepath.Join(esp, keyFilePath))
	if err := sbtpmSealedKeyObjectWriteAtomic(k, w); err != nil {
		return fmt.Errorf("cannot write updated sealed key object: %w", err)
	}

	return nil
}

// TrustCurrentBoot adds the assets used in the current boot to the list of boot
// assets trusted for adding to PCR profiles with ResealKey. It works by mapping
// EV_EFI_BOOT_SERVICES_APPLICATION events from the TCG log to files stored in the
// ESP.
func TrustCurrentBoot(assets *TrustedAssets, esp string) error {
	f, err := appFs.Open("/sys/kernel/security/tpm0/binary_bios_measurements")
	if err != nil {
		return err
	}
	defer f.Close()

	eventLog, err := tcglog.ReadLog(f, &tcglog.LogOptions{})
	if err != nil {
		return fmt.Errorf("cannot read TCG log: %v", err)
	}

	for _, event := range eventLog.Events {
		if event.PCRIndex != 4 {
			continue
		}
		if event.EventType != tcglog.EventTypeEFIBootServicesApplication {
			continue
		}

		data, ok := event.Data.(*tcglog.EFIImageLoadEvent)
		if !ok {
			log.Println("Invalid event data for EV_EFI_BOOT_SERVICES_APPLICATION event")
			continue
		}

		fpdp, ok := data.DevicePath[len(data.DevicePath)-1].(efi.FilePathDevicePathNode)
		if !ok {
			// Ignore application not stored in a filesystem
			continue
		}

		components := strings.Split(string(fpdp), "\\")
		path := strings.Join(components, string(os.PathSeparator))

		err := func() error {
			f, err := appFs.Open(filepath.Join(esp, path))
			switch {
			case os.IsNotExist(err):
				log.Println("Missing file:", filepath.Join(esp, path))
				return nil
			case err != nil:
				return err
			}

			peHashMatch := false

			hf, err := newHashedFile(f, assets.alg(), func(leafHashes [][]byte) {
				if !peHashMatch {
					return
				}
				assets.trustLeafHashes(leafHashes)
			})
			if err != nil {
				f.Close()
				return err
			}
			defer hf.Close()

			digest, err := efiComputePeImageDigest(crypto.SHA256, hf, hf.Size())
			if err != nil {
				return fmt.Errorf("cannot compute PE image hash: %v", err)
			}
			if bytes.Equal(digest, event.Digests[tpm2.HashAlgorithmSHA256]) {
				peHashMatch = true
			}

			return nil
		}()
		if err != nil {
			return err
		}
	}

	return nil
}
