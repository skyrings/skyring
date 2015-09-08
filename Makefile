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

pybuild:
	@echo "Doing $@"
	@cd python; python setup.py build

build: getdeps verifiers pybuild test
	@echo "Doing $@"
	@go build

pyinstall:
	@echo "Doing $@"
	@cd python; python setup.py install

saltinstall:
	@echo "Doing $@"
	@cp -fv salt/* /srv/salt/

install: build pyinstall saltinstall
	@echo "Doing $@"
	@go install
