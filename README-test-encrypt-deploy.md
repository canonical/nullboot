# Local provisioning and booting

This tutorial describes how to provision an Ubuntu image with the nullbootctl:
- get a base cloud image
- setup swtpm
- encrypt
- deploy
- boot with QEMU

Prerequisites:
- Linux host
- root privileges (required to create NBD devices, use /dev/mapper/control and mount block devices)
- `cryptsetup` (>= 2.2.0)
- `qemu-system-x86`, `qemu-utils`
- `cloud-guest-utils` (for growpart)
- `swtpm`, provides a vTPM implementation
- `tpm2-tools`, client library used to communicate with the vTPM
- `virt-firmware`, provides the virt-fw-vars tool
- `ovmf` or `ovmf-generic`, for OVMF firmware and variables
- python3
- for SSH access into the VM:
	* a Launchpad account
	* `cloud-image-utils`

## Build

See [README.md](README.md#build).


## Setup

Get the latest Noble (Ubuntu 24.04) server cloud image:
```bash
wget http://cloud-images.ubuntu.com/azure/autopkgtest/tpm-fde/noble/latest/noble-server-cloudimg-amd64-azure.fde.vhd.tar.gz
tar xvf noble-server-cloudimg-amd64-azure.fde.vhd.tar.gz
ln -s livecd.ubuntu-cpc.azure.fde.vhd input.vhd
```

Ensure the kernel module `nbd` is installed:
```bash
sudo modprobe nbd
```


## Make a script to convert UEFI variables
```bash
cat << EOF > convert-to-uefi-config
# This script basically converts:
#     { "version": 2, "variables": [ { "name": "PK", "data": "a159c0a5e494a74a87b5ab155c..." },... ] }
# to: { "PK": "oVnApeSUp0qHtasVXCvwcm...",...}
# Input: vars.json
# Output: uefi-config.json
import base64
import json

with open("vars.json") as f:
    variables = {v["name"]: v for v in json.load(f)["variables"]}

config = {
    name: base64.b64encode(bytes.fromhex(variables[name]["data"])).decode("ascii")
    for name in ("PK", "KEK", "db", "dbx")
}
with open("uefi-config.json", "w") as f:
    json.dump(config, f, indent=2)
EOF
```

## Prepare `OVMF_VARS_4M.ms.fd`

Copy UEFI variables that will be fed to QEMU:
```bash
cp /usr/share/OVMF/OVMF_VARS_4M.ms.fd .
```
(we are only interested in `PK`, `KEK`, `db`, `dbx`, as they enter in the guest
measurement used to protect disk encryption keys)

Start a virtual machine (VM) for the sole purpose of having
`OVMF_CODE_4M.ms.fd` run and update these UEFI variables (through a minimal boot).
This step is needed if your host is on Ubuntu 26.04.1 but not on Ubuntu 24.04:
```bash
# get it killed after 3 seconds, which should be enough for OVMF_CODE_4M.ms.fd to execute
timeout 3 qemu-system-x86_64 -cpu host \
                   -machine type=q35,accel=kvm \
                   -m 2048 \
                   -nographic \
                   -drive if=virtio,format=raw,file=input.vhd,readonly=true \
                   -drive if=pflash,format=raw,file=/usr/share/OVMF/OVMF_CODE_4M.ms.fd,readonly=true \
                   -drive if=pflash,format=raw,file=OVMF_VARS_4M.ms.fd
```

Get the UEFI variables to the appropriate format: 
```bash
virt-fw-vars -i OVMF_VARS_4M.ms.fd --output-json vars.json
python3 convert-to-uefi-config
```

## Setup swtpm

Start a vTPM with swtpm (and keep it running until the end of the page):
```bash
mkdir /tmp/mytpm0
swtpm socket --server type=unixio,path=/tmp/mytpm0/swtpm-sock \
             --ctrl type=unixio,path=/tmp/mytpm0/swtpm-sock.ctrl,mode=0600 \
             --tpmstate dir=/tmp/mytpm0 \
             --tpm2 \
             --flags not-need-init,startup-clear
```

Initialize the vTPM with a SRK (Storage Root Key):
```bash
export TPM2TOOLS_TCTI="swtpm:path=/tmp/mytpm0/swtpm-sock"
tpm2_createprimary -c srk.ctx
tpm2_readpublic -c srk.ctx -o srk.pub
```

## Encrypt
```bash
sudo nullbootctl encrypt -o encrypted.vhd \
                         --override-datasources "NoCloud" \
                         input.vhd

```
The encrypted image has now an encrypted disk, with keys stored in plaintext in
the LUKS header.

## Deploy
```bash
sudo nullbootctl deploy --srk-pub srk.pub \
                        --uefi-config uefi-config.json \
                        --add-efi-secure-boot-profile \
                        encrypted.vhd

# This is the final image
mv encrypted.vhd final.vhd
sudo chmod a+rw final.vhd
```
The final image has now an encrypted disk, with new keys, which are protected
by the vTPM.

## Boot

For SSH access, prepare `seed.img`:
```bash
LAUNCHPAD_USERNAME=<your-lp-account-username>
cat << EOF > user-data.yaml
#cloud-config
ssh_import_id: $LAUNCHPAD_USERNAME
EOF

cloud-localds seed.img user-data.yaml
```

Boot a VM with the final image:
```bash
qemu-system-x86_64 -cpu host \
                   -machine type=q35,accel=kvm \
                   -m 2048 \
                   -nographic \
                   -netdev id=net00,type=user,hostfwd=tcp::2222-:22 \
                   -device virtio-net-pci,netdev=net00 \
                   -drive if=virtio,format=raw,file=final.vhd \
                   -drive if=virtio,format=raw,file=seed.img \
                   -drive if=pflash,format=raw,file=/usr/share/OVMF/OVMF_CODE_4M.ms.fd,readonly=true \
                   -drive if=pflash,format=raw,file=OVMF_VARS_4M.ms.fd \
                   -chardev socket,id=chrtpm,path=/tmp/mytpm0/swtpm-sock.ctrl \
                   -tpmdev emulator,id=tpm0,chardev=chrtpm \
                   -device tpm-tis,tpmdev=tpm0
```
... which will boot you to:
```
ubuntu login:
```
(`control-a`, `x` to exit)

And you can ssh into the VM with:
```
ssh ubuntu@127.0.0.1 -p 2222
```
(add option `-i ...` to select a specific key)
