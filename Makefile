# store the current working directory
CWD := $(shell pwd)
BASEPKG := github.com/skyrings/skyring
BASEDIR := $(GOPATH)/src/$(BASEPKG)
BASEDIR_PARENTDIR := $(shell dirname $(BASEDIR))
BIGFINDIR := $(BASEDIR_PARENTDIR)/bigfin
PRINT_STATUS = export EC=$$?; cd $(CWD); if [ "$$EC" -eq "0" ]; then printf "SUCCESS!\n"; else exit $$EC; fi

BUILDS    := .build
DEPLOY    := $(BUILDS)/deploy
VERSION   := 1.0
TARDIR    := skyring-$(VERSION)
RPMBUILD  := $(HOME)/rpmbuild/

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
	@go get -t ./...

verifiers: getdeps vet fmt lint

vet:
	@echo "Doing $@"
	@$(PWD)/build-aux/vet .

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
	@cd backend/salt/python; python setup.py build

build: getdeps verifiers pybuild test
	@echo "Doing $@"
	@go build

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
	@go install

rpm:
	@echo "target: rpm"
	@echo  "  ...building rpm $(V_ARCH)..."
	if [ ! -f 'skyring' ] && [ ! -x 'skyring' ] ; then make build ; fi
	rm -fr $(BUILDS) $(HOME)/$(BUILDS)
	mkdir -p $(DEPLOY)/latest $(HOME)/$(BUILDS)
	cd $(BIGFINDIR)/backend/salt/python; \
	python setup.py bdist --formats=rpm
	cp -f $(BIGFINDIR)/backend/salt/python/dist/skyring*noarch.rpm $(DEPLOY)/latest/
	cp -fr $(BASEDIR) $(HOME)/$(BUILDS)/$(TARDIR)
	cd $(HOME)/$(BUILDS); \
	cp -f $(TARDIR)/skyring $(TARDIR)/_skyring; \
	cp -f $(TARDIR)/backend/salt/python/setup.py $(TARDIR); \
	cp -fr $(TARDIR)/backend/salt/python/skyring $(TARDIR)/; \
	tar -zcf skyring-$(VERSION).tar.gz $(TARDIR); \
	cp skyring-$(VERSION).tar.gz $(RPMBUILD)/SOURCES
	# Cleaning the work directory
	rm -fr $(HOME)/$(BUILDS)
	rpmbuild -ba skyring.spec
	$(PRINT_STATUS); \
	if [ "$$EC" -eq "0" ]; then \
		FILE=$$(readlink -f $$(find $(RPMBUILD)/RPMS -name skyring-$(VERSION)*.rpm)); \
		cp -f $$FILE $(DEPLOY)/latest/; \
		printf "\nThe Skyring RPMs are located at:\n\n"; \
		printf "   $(DEPLOY)/latest\n\n\n\n"; \
	fi
