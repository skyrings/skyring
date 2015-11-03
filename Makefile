# store the current working directory
CWD := $(shell pwd)
BINDIR := $(GOPATH)/bin
PRINT_STATUS = export EC=$$?; cd $(CWD); if [ "$$EC" -eq "0" ]; then printf "SUCCESS!\n"; else exit $$EC; fi

VERSION   := 0.0.1
TARDIR    := skyring-$(VERSION)
RPMBUILD  := $(HOME)/rpmbuild/
SKYRING_BUILD  := $(HOME)/.skyring_build
SKYRING_BUILD_SRC  := $(SKYRING_BUILD)/golang/gopath/src/github.com/skyrings/skyring

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
	@GO15VENDOREXPERIMENT=1 glide -q up 2> /dev/null

build-init:
	rm -fr $(SKYRING_BUILD_SRC)
	mkdir $(SKYRING_BUILD_SRC) -p
	cp -ai $(CWD)/* $(SKYRING_BUILD_SRC)/

gobuild: verifiers vendor-update pybuild test
	@echo "Doing $@"
	@GO15VENDOREXPERIMENT=1 go build

build:  build-init
	cd $(SKYRING_BUILD_SRC); \
	export GOROOT=/usr/lib/golang/; \
	export GOPATH=$(SKYRING_BUILD)/golang/gopath; \
	export PATH=$(PATH):$(GOPATH)/bin:$(GOROOT)/bin; \
	make gobuild

pyinstall:
	@echo "Doing $@"
	@cd backend/salt/python; python setup.py --quiet install --user
	@echo "INFO: You should set PYTHONPATH make it into effect"
	@echo "INFO: or run skyring by \`PYTHONPATH=~/.local/lib/python2.7/site-packages skyring\`"

saltinstall:
	@echo "Doing $@"
	@if ! cp -f salt/* /srv/salt/ 2>/dev/null; then \
		echo "ERROR: unable to install salt files. Install them manually by"; \
		echo "sudo cp -f backend/salt/sls/* /srv/salt/"; \
	fi

install: build pyinstall saltinstall
	@echo "Doing $@"
	@GO15VENDOREXPERIMENT=1 go install

rpm:    build
	@echo "target: rpm"
	@echo  "  ...building rpm $(V_ARCH)..."
	rm -fr $(SKYRING_BUILD_SRC)/skyring
	mkdir -p $(RPMBUILD)/SOURCES
	cd $(SKYRING_BUILD_SRC); \
	tar -zcf skyring-$(VERSION).tar.gz $(SKYRING_BUILD_SRC)/../$(TARDIR); \
	cp $(SKYRING_BUILD_SRC)/skyring-$(VERSION).tar.gz $(RPMBUILD)/SOURCES; \
	rpmbuild -ba skyring.spec
	$(PRINT_STATUS); \
	if [ "$$EC" -eq "0" ]; then \
		FILE=$$(readlink -f $$(find $(RPMBUILD)/RPMS -name skyring-$(VERSION)*.rpm)); \
		cp -f $$FILE $(SKYRING_BUILD)/; \
		printf "\nThe Skyring RPMs are located at:\n\n"; \
		printf "   $(SKYRING_BUILD)/\n\n\n\n"; \
	fi
