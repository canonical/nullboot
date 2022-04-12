module github.com/canonical/nullboot

go 1.16

require (
	github.com/canonical/go-efilib v0.3.1-0.20220314143719-95d50e8afc82
	github.com/canonical/go-tpm2 v0.1.0
	github.com/canonical/tcglog-parser v0.0.0-20220314144800-471071956aa1
	github.com/knqyf263/go-deb-version v0.0.0-20190517075300-09fca494f03d
	github.com/snapcore/go-gettext v0.0.0-20201130093759-38740d1bd3d2 // indirect
	github.com/snapcore/secboot v0.0.0-20220406084634-6e724131009b
	github.com/snapcore/snapd v0.0.0-20220411132918-d69f2ac36bd2 // indirect
	github.com/spf13/afero v1.8.2
	go.mozilla.org/pkcs7 v0.0.0-20210826202110-33d05740a352 // indirect
	golang.org/x/crypto v0.0.0-20220411220226-7b82a4e95df4 // indirect
	golang.org/x/net v0.0.0-20220412020605-290c469a71a5 // indirect
	golang.org/x/sys v0.0.0-20220412071739-889880a91fd5
	golang.org/x/text v0.3.7
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/snapcore/secboot => github.com/chrisccoulson/secboot v0.0.0-20211101133820-41f32b803753
