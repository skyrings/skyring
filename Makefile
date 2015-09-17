all: install

checkdeps:
	@echo "Doing $@"
	@bash $(PWD)/build-aux/checkdeps.sh

getdeps: checkdeps getversion
	@echo "Doing $@"
	@go get github.com/golang/lint/golint
	@go get -t ./...

getversion:
	@echo "Doing $@"
	@bash $(PWD)/build-aux/pkg-version.sh $(PWD)/version.go

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

build: getdeps verifiers test
	@echo "Doing $@"
	@go build

install: build
	@echo "Doing $@"
	@go install
