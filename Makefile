
# Build command line programs
build: nullbootctl
.PHONY: FORCE build
FORCE:

%: cmd/%/main.go FORCE
	go build -o $@ ./cmd/$@

fmt:
	go fmt ./...

check: build
	go test -v -coverprofile=profile.cov ./...


