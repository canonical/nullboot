module github.com/canonical/nullboot

go 1.18

require (
	github.com/canonical/go-efilib v0.3.1-0.20220324150059-04e254148b45
	github.com/canonical/go-tpm2 v0.1.0
	github.com/canonical/tcglog-parser v0.0.0-20220314144800-471071956aa1
	github.com/knqyf263/go-deb-version v0.0.0-20190517075300-09fca494f03d
	github.com/snapcore/secboot v0.0.0-20220406084634-6e724131009b
	github.com/spf13/afero v1.10.0
	golang.org/x/sys v0.13.0
	golang.org/x/text v0.14.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
)

require (
	github.com/canonical/go-sp800.108-kdf v0.0.0-20210315104021-ead800bbf9a0 // indirect
	github.com/canonical/go-sp800.90a-drbg v0.0.0-20210314144037-6eeb1040d6c3 // indirect
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/kr/pretty v0.2.2-0.20200810074440-814ac30b4b18 // indirect
	github.com/kr/text v0.1.0 // indirect
	github.com/snapcore/go-gettext v0.0.0-20201130093759-38740d1bd3d2 // indirect
	github.com/snapcore/snapd v0.0.0-20220411132918-d69f2ac36bd2 // indirect
	go.mozilla.org/pkcs7 v0.0.0-20210826202110-33d05740a352 // indirect
	golang.org/x/crypto v0.14.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f // indirect
	gopkg.in/retry.v1 v1.0.3 // indirect
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	maze.io/x/crypto v0.0.0-20190131090603-9b94c9afe066 // indirect
)

replace github.com/snapcore/secboot => github.com/chrisccoulson/secboot v0.0.0-20211101133820-41f32b803753
