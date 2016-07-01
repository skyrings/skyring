# store the current working directory
CWD := $(shell pwd)
PRINT_STATUS = export EC=$$?; cd $(CWD); if [ "$$EC" -eq "0" ]; then printf "SUCCESS!\n"; else exit $$EC; fi

VERSION   := 0.0.30
RELEASE   := 1
TARDIR    := ../skyring-$(VERSION)
RPMBUILD  := $(HOME)/rpmbuild
SKYRING_BUILD  := $(HOME)/.skyring_build
SKYRING_BUILD_SRC  := $(SKYRING_BUILD)/golang/gopath/src/github.com/skyrings/skyring
SKYRING_BUILD_TARDIR := $(SKYRING_BUILD)/golang/gopath/src/github.com/skyrings/skyring/$(TARDIR)

bzip-selinux-policies:
	@cd selinux; \
	rm -f *.pp.bz2 tmp; \
	make -f /usr/share/selinux/devel/Makefile; \
	bzip2 -9 skyring.pp; \
	bzip2 -9 salt.pp; \
        bzip2 -9 carbon.pp

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

build: verifiers pybuild test
	@echo "Doing $@"
	@GO15VENDOREXPERIMENT=1 go build

build-special:
	rm -fr $(SKYRING_BUILD_SRC) $(SKYRING_BUILD)
	mkdir $(SKYRING_BUILD_SRC) -p
	cp -ai $(CWD)/* $(SKYRING_BUILD_SRC)/
	cd $(SKYRING_BUILD_SRC); \
	export GOROOT=/usr/lib/golang/; \
	export GOPATH=$(SKYRING_BUILD)/golang/gopath; \
	cp -r $(SKYRING_BUILD_SRC)/vendor/* $(SKYRING_BUILD)/golang/gopath/src/ ; \
	export PATH=$(PATH):$(GOPATH)/bin:$(GOROOT)/bin; \
	go build
	cp $(SKYRING_BUILD_SRC)/skyring $(CWD)

pyinstall:
	@echo "Doing $@"
	@if [ "$$USER" == "root" ]; then \
		cd backend/salt/python; python setup.py --quiet install --root / --force; cd -; \
	else \
		cd backend/salt/python; python setup.py --quiet install --user; cd -; \
		echo "    INFO: You should set PYTHONPATH make it into effect"; \
		echo "    INFO: or run skyring by \`PYTHONPATH=~/.local/lib/python2.7/site-packages skyring\`"; \
	fi

confinstall:
	@echo "Doing $@"
	@if [ "$$USER" == "root" ]; then \
		[ -d /etc/skyring ] || mkdir -p /etc/skyring; \
		[[ -f /etc/skyring/skyring.conf ]] || cp conf/sample/skyring.conf.sample /etc/skyring/skyring.conf; \
		cp event/skyring.evt /etc/skyring; \
	else \
		echo "ERROR: unable to install conf files. Install them manually by"; \
		echo "    sudo cp conf/sample/skyring.conf.sample /etc/skyring/skyring.conf"; \
		echo "    sudo cp event/skyring.evt /etc/skyring"; \
	fi

saltinstall:
	@echo "Doing $@"
	@if [ "$$USER" == "root" ]; then \
		[ -d /srv/salt ] || mkdir -p /srv/salt; \
		[ -d /srv/salt/collectd ] || mkdir -p /srv/salt/collectd; \
		[ -d /srv/salt/collectd/files ] || mkdir -p /srv/salt/collectd/files; \
		[ -d /srv/salt/template ] || mkdir -p /srv/salt/template; \
		cp backend/salt/sls/*.* /srv/salt; \
		cp backend/salt/sls/collectd/*.* /srv/salt/collectd; \
		cp backend/salt/conf/collectd/* /srv/salt/collectd/files; \
		cp backend/salt/template/* /srv/salt/template; \
	else \
		echo "ERROR: unable to install salt files. Install them manually by"; \
		echo "    sudo cp backend/salt/sls/*.* /srv/salt"; \
		echo "    sudo cp backend/salt/sls/collectd/*.* /srv/salt/collectd"; \
		echo "    sudo cp backend/salt/conf/collectd/* /srv/salt/collectd/files"; \
		echo "    sudo cp backend/salt/template/* /srv/salt/template"; \
	fi

install: build pyinstall confinstall saltinstall
	@echo "Doing $@"
	@GO15VENDOREXPERIMENT=1 go install

dist:
	@echo "Doing $@"
	rm -fr $(TARDIR)
	mkdir -p $(TARDIR)
	rsync -r --exclude .git/ $(CWD)/ $(TARDIR)
	tar -zcf $(TARDIR).tar.gz $(TARDIR);

rpm:    dist
	@echo "Doing $@"
	rm -rf $(RPMBUILD)/SOURCES
	mkdir -p $(RPMBUILD)/SOURCES
	cp ../skyring-$(VERSION).tar.gz $(RPMBUILD)/SOURCES; \
	rpmbuild -ba skyring.spec
	$(PRINT_STATUS); \
	if [ "$$EC" -eq "0" ]; then \
		FILE=$$(readlink -f $$(find $(RPMBUILD)/RPMS -name skyring-$(VERSION)*.rpm)); \
		cp -f $$FILE $(SKYRING_BUILD)/; \
		printf "\nThe Skyring RPMs are located at:\n\n"; \
		printf "   $(SKYRING_BUILD)/\n\n\n\n"; \
	fi
