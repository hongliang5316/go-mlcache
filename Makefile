.PHONY : vet version test

vet: version
	go vet ./...

version:
	go version

test: vet
	GORACE=history_size=7 gotest -gcflags='-l' -race -v ./...
