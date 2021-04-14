.PHONY : vet version test

vet: version
	go vet ./...

ineffassign:
	ineffassign .

version:
	go version

test: ineffassign vet
	GORACE=history_size=7 gotest -count=1 -gcflags='-l' -race -v ./...
