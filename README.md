A boot manager that isn't
=========================

nullboot is a boot manager for environments that do not need a boot manager.
Instead of running a boot manager at boot, it directly manages the UEFI boot
entries for you.

Use cases
---------

Usage:
```
1. nullbootctl install [OPTIONS]
2. nullbootctl recovery-key [OPTIONS]
3. nullbootctl encrypt [OPTIONS]
```

### nullbootctl install

Install shim and UKI in EFI System Partition.

More specifically:
- Install shim and UKI (Unified Kernel Image) to the EFI System Partition (ESP,
  ie: /boot/efi)
- Update the TPM sealing policy of the LUKS passphrase
- Set EFI variable BootNext to point to the newly installed shim (and kernel?) (TODO)

It is intended to be called upon shim and UKI update.

See `nullbootctl install --help` for options:
```
Usage:
  nullbootctl install [OPTIONS]

Help Options:
  -h, --help                      Show this help message

Options:
          --no-tpm                Do not do any resealing with the TPM
          --no-efivars            Do not use or update the EFI variables.
                                  Disables kernel fallback mechanism
          --output-json=<file>    JSON file to write. Disables writing real EFI
                                  variables and enablement of the kernel
                                  fallback mechanism
          --no-boot-next          Disables use of BootNext. This flag must be
                                  disabled in order to upgrade to a new kernel
                                  version.
```


### nullbootctl recovery-key

Manage recovery keys (LUKS passphrases) of a LUKS container.

This shall setup, list, delete and verify recovery keys for a LUKS container.

See `nullbootctl recovery-key --help` for options:
```
Usage:
  nullbootctl recovery-key [OPTIONS]

Create, list, delete or verify a LUKS2 disk encryption recovery key.

Help Options:
  -h, --help                   Show this help message

Options:
          --create             Create and set a recovery key
          --list               List recovery keys
          --delete             Delete a recovery key
          --verify             Verify a recovery key
          --device=<device>    Device of the encrypted volume (default:
                               /dev/disk/by-label/cloudimg-rootfs-enc)
          --name=<name>        Name of the recovery key (default: recovery-0001)
```

Example:
```
# nullbootctl recovery-key --create --name R1
Creating recovery key 'recovery-0001' in '/dev/disk/by-label/cloudimg-rootfs-enc'
38785-16377-60025-09490-45202-37955-36799-29994

# nullbootctl recovery-key --create --name R2
Creating recovery key 'R2' in '/dev/disk/by-label/cloudimg-rootfs-enc'
43392-00597-12755-38614-06919-19535-19552-05646

# nullbootctl recovery-key --list
Listing recovery keys in '/dev/disk/by-label/cloudimg-rootfs-enc'
R1
R2

# echo 38785-16377-60025-09490-45202-37955-36799-29994 | nullbootctl recovery-key --verify
success
```

### nullbootctl encrypt

Perform the pre-encryption of a Ubuntu cloud image without protecting the key
to a specific guest instance. The key is stored in cleartext inside the LUKS
metadata, and can be used to create a pool of pre-encrypted images.


Build
-----
```
$ go build -o . ./...
```

Licensing
---------
This program is free software: you can redistribute it and/or modify it under
the terms of the GNU General Public License version 3, as published by the Free
Software Foundation.

This program is distributed in the hope that it will be useful, but WITHOUT ANY
WARRANTY; without even the implied warranties of MERCHANTABILITY, SATISFACTORY
QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public
License for more details.

You should have received a copy of the GNU General Public License along with
this program.  If not, see <http://www.gnu.org/licenses/>.
