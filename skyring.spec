%define name skyring
%define go_version 1.0
%define release 0

%global _etcdir /etc
%global _usrlibdir /usr/lib

Name: %{name}
Version: %{go_version}
Release: %{release}%{?dist}
Summary: Modern extensible web-based storage management platform
Source0: %{name}-%{go_version}.tar.gz
License: Apache-2.0
Group: Applications/Storage
BuildRoot: %{_tmppath}/%{name}-%{go_version}-%{release}-buildroot
Url: github.com/skyrings/skyring

Requires: git
Requires: golang
Requires: salt-master
Requires: python-devel
Requires: python-cpopen
Requires: python-paramiko
Requires: hg
Requires: mongodb
Requires: mongodb-server
Requires: openldap-devel
Requires: ceph

%description
SkyRing is a modern, extensible web-based storage management platform
which represents the future of storage management. SkyRing deploys,
manages and monitors various storage technologies and provides
a pluggable framework for tomorrow’s SDS technologies.
SkyRing integrates best-of-breed open source components at its core,
and integrates with the broader management stack.

%prep
%setup -n %{name}-%{go_version}

%build
python setup.py build

%install
rm -rf $RPM_BUILD_ROOT
install -D _skyring $RPM_BUILD_ROOT/usr/bin/skyring
install -D skyring-setup.sh $RPM_BUILD_ROOT/usr/bin/skyring-setup.sh
install -D conf/sample/skyring.conf.sample $RPM_BUILD_ROOT/etc/skyring/skyring.conf
install -D conf/sample/authentication.conf.sample $RPM_BUILD_ROOT/etc/skyring/authentication.conf
install -D conf/sample/providers.d/ceph.conf.sample $RPM_BUILD_ROOT/etc/skyring/providers.d/ceph.conf
install -D conf/skyring_salt_master.conf $RPM_BUILD_ROOT/etc/salt/master.d/skyring.conf
install -m 755 -d $RPM_BUILD_ROOT/srv/salt
install -m 755 -d $RPM_BUILD_ROOT/var/log/skyring
install -D backend/salt/sls/* $RPM_BUILD_ROOT/srv/salt/
install -d $RPM_BUILD_ROOT/%{python2_sitelib}/skyring
install -D backend/salt/python/skyring/* $RPM_BUILD_ROOT/%{python2_sitelib}/skyring/

%post

%preun

%clean
rm -rf "$RPM_BUILD_ROOT"

%files
%attr(0755, root, root) /usr/bin/skyring
%attr(0755, root, root) /usr/bin/skyring-setup.sh
%_etcdir/skyring/*
%_etcdir/skyring/providers.d/*
%_etcdir/salt/master.d/skyring.conf
%{python2_sitelib}/skyring/*
/srv/salt/*
/var/log/skyring

%changelog
* Thu Dec 03 2015 <tjeyasin@redhat.com>
- Initial build.
