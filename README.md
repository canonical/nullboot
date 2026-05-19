A boot manager that isn't
=========================

nullboot is a boot manager for environments that do not need a boot manager.
Instead of running a boot manager at boot, it directly manages the UEFI boot
entries for you.

Use cases
---------

Usage:
1. nullbootctl
2. nullbootctl -no-tpm -output-json FILE
3. nullbootctl -no-boot-next
4. nullbootctl recovery-key [OPTIONS]

### nullbootctl

Finalize shim and UKI installation.

Without any argument, nullbootctl shall:
- Install shim and UKI (Unified Kernel Image) to the EFI System Partition (ESP,
  ie: /boot/efi)
- Update the sealing policy of the LUKS passphrase in the TPM
- Set EFI variable BootNext to point to the newly installed shim and kernel

It is intended to be called upon shim and UKI update.


### nullbootctl -no-tpm -output-json FILE

- Export EFI variables Boot#### and BootOrder.
- Install shim & UKI
- Update the sealing policy of the LUKS passphrase in the TPM

This shall create a JSON file with the EFI variables Boot#### and BootOrder, as
expected on the first boot.


### nullbootctl -no-boot-next

Commit EFI variable BootOrder after shim and UKI update.

This shall:
- Install shim and UKI (possibly a no-operation)
- Update the sealing policy of the LUKS passphrase in the TPM (possibly a
  no-operation)
- Commit shim and UKI by updating EFI variable BootOrder (when the system
  boots on EFI variable BootNext)
- Remove old kernel


### nullbootctl recovery-key [OPTIONS]

Manage recovery keys (LUKS passphrases) of a LUKS container.

This shall setup, list or delete a recovery key for the LUKS container.

Options:
  --create [--device DEVICE] [--name NAME]
  --delete [--device DEVICE] [--name NAME]
  --list   [--device DEVICE]

Example:
```
# nullbootctl recovery-key --create
Creating recovery key 'recovery-0001' in '/dev/disk/by-label/cloudimg-rootfs-enc'
18466-30786-51485-64513-29543-52270-35959-33619

# nullbootctl recovery-key --list
Listing recovery keys in '/dev/disk/by-label/cloudimg-rootfs-enc'
recovery-0001

# nullbootctl recovery-key --delete
Deleting recovery key 'recovery-0001' in '/dev/disk/by-label/cloudimg-rootfs-enc'
Cannot delete recovery key: cannot kill last remaining slot
```

Build
-----
```
$ go build -o . ./...
```

Execute
-------
```
$ ./nullbootctl -h
2026/05/19 09:40:54 usage:

1. nullbootctl
2. nullbootctl -no-tpm -output-json FILE
3. nullbootctl -no-boot-next
4. nullbootctl recovery-key [OPTIONS]

Commands:

  recovery-key
	  Manage FDE recovery keys (LUKS passphrases).
	  Options:
		--create [--device DEVICE] [--name NAME]
		--delete [--device DEVICE] [--name NAME]
		--list   [--device DEVICE]
```

Unit test
---------
```
$ go test -v -coverprofile=profile.cov ./... 
$ echo $?
0
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
