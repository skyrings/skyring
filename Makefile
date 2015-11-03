# store the current working directory
CWD := $(shell pwd)
BINDIR := $(GOPATH)/bin
PRINT_STATUS = export EC=$$?; cd $(CWD); if [ "$$EC" -eq "0" ]; then printf "SUCCESS!\n"; else exit $$EC; fi

VERSION   := 1.0
TARDIR    := skyring-$(VERSION)
RPMBUILD  := $(HOME)/rpmbuild/
SKYBUILD  := $(HOME)/.skyring_build
DEPLOY    := $(SKYBUILD)/deploy
SKYTARBLDSRC := $(SKYBUILD)/golang/gopath/src/github.com/skyrings/$(TARDIR)
SKYBUILDSRC  := $(SKYBUILD)/golang/gopath/src/github.com/skyrings/skyring

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
	rm -fr $(SKYBUILDSRC) $(SKYTARBLDSRC) $(DEPLOY)
	mkdir $(DEPLOY) $(SKYBUILDSRC) -p
	cp -ai $(CWD)/* $(SKYBUILDSRC)/

gobuild: verifiers vendor-update pybuild test
	@echo "Doing $@"
	@GO15VENDOREXPERIMENT=1 go build

build:  build-init
	cd $(SKYBUILDSRC); \
	export GOROOT=/usr/lib/golang/; \
	export GOPATH=$(SKYBUILD)/golang/gopath; \
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
	rm -fr $(SKYBUILDSRC)/skyring
	mkdir -p $(RPMBUILD)/SOURCES
	cp -ai $(SKYBUILDSRC) $(SKYTARBLDSRC)
	cp $(BINDIR)/skyring $(SKYTARBLDSRC)
	cd $(SKYBUILDSRC); \
	tar -zcf skyring-$(VERSION).tar.gz $(SKYBUILDSRC)/../$(TARDIR); \
	cp $(SKYBUILDSRC)/skyring-$(VERSION).tar.gz $(RPMBUILD)/SOURCES; \
	rpmbuild -ba skyring.spec
	$(PRINT_STATUS); \
	if [ "$$EC" -eq "0" ]; then \
		FILE=$$(readlink -f $$(find $(RPMBUILD)/RPMS -name skyring-$(VERSION)*.rpm)); \
		cp -f $$FILE $(DEPLOY)/; \
		printf "\nThe Skyring RPMs are located at:\n\n"; \
		printf "   $(DEPLOY)/\n\n\n\n"; \
	fi
