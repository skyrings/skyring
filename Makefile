all: install

checkdeps:
	@echo "Doing $@"
	@bash $(PWD)/build-aux/checkdeps.sh

getversion:
	@echo "Doing $@"
	@bash $(PWD)/build-aux/pkg-version.sh $(PWD)/version.go

getdeps: checkdeps getversion
	@echo "Doing $@"
	@go get github.com/golang/lint/golint
	@go get github.com/Masterminds/glide

verifiers: getdeps vet fmt lint

vet:
	@echo "Doing $@"
	@bash $(PWD)/build-aux/run-vet.sh

fmt:
	@echo "Doing $@"
	@bash $(PWD)/build-aux/gofmt.sh

lint:
	@echo "Doing $@"
	@golint .

test:
	@echo "Doing $@"
	@GO15VENDOREXPERIMENT=1 go test $$(GO15VENDOREXPERIMENT=1 glide nv)

pybuild:
	@echo "Doing $@"
	@cd backend/salt/python; python setup.py build

vendor-update:
	@echo "Updating vendored packages"
	@GO15VENDOREXPERIMENT=1 glide -q up

build: verifiers vendor-update pybuild test
	@echo "Doing $@"
	@GO15VENDOREXPERIMENT=1 go build

pyinstall:
	@echo "Doing $@"
	@cd backend/salt/python; python setup.py --quiet install --user
	@echo "INFO: You should set PYTHONPATH make it into effect"
	@echo "INFO: or run skyring by \`PYTHONPATH=~/.local/lib/python2.7/site-packages skyring\`"

saltinstall:
	@echo "Doing $@"
	@if ! cp -f salt/* /srv/salt/ 2>/dev/null; then \
		echo "ERROR: unable to install salt files. Install them manually by"; \
		echo "sudo cp -f salt/* /srv/salt/"; \
	fi

install: build pyinstall saltinstall
	@echo "Doing $@"
	@GO15VENDOREXPERIMENT=1 go install
