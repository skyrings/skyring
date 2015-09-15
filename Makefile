all: install

checkdeps:
	@echo "Doing $@"
	@bash $(PWD)/build-aux/checkdeps.sh

getdeps: checkdeps
	@echo "Doing $@"
	@go get github.com/golang/lint/golint
	@go get -t ./...

verifiers: getdeps vet fmt lint

vet:
	@echo "Doing $@"
	@go tool vet .

fmt:
	@echo "Doing $@"
	@bash $(PWD)/build-aux/gofmt.sh

lint:
	@echo "Doing $@"
	@golint .

test:
	@echo "Doing $@"
	@go test -v ./...

getversion:
	@echo "Doing $@"
	@bash $(PWD)/build-aux/pkg-version.sh $(PWD)/version.go

build: getdeps verifiers getversion test
	@echo "Doing $@"
	@go build

install: build
	@echo "Doing $@"
	@go install
