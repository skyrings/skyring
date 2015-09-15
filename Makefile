all: install

checkdeps:
	@echo "Checking build environment"
	@bash $(PWD)/buildscripts/checkdeps.sh

getdeps: checkdeps
	go get github.com/golang/lint/golint
	go -t get ./...

verifiers: getdeps vet fmt lint

vet:
	go tool vet .

fmt:
	@echo "Checking go format"
	find . -name "*.go" | xargs gofmt -l -s

lint:
	@echo "Running $@"
	golint .

test:
	go test -v ./...

getversion:
	@bash $(PWD)/buildscripts/pkg-version.sh $(PWD)/version.go

build: getdeps verifiers getversion test
	go build

install: build
	go install
