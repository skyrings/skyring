%define pkg_name skyring
%define pkg_version 0.0.1
%define pkg_release 1

Name: %{pkg_name}
Version: %{pkg_version}
Release: %{pkg_release}%{?dist}
Summary: Modern extensible web-based storage management platform
Source0: %{pkg_name}-%{pkg_version}.tar.gz
License: Apache-2.0
Group: Applications/Storage
BuildRoot: %{_tmppath}/%{pkg_name}-%{pkg_version}-%{pkg_release}-buildroot
Url: github.com/skyrings/skyring

BuildRequires: golang
BuildRequires: python-devel
BuildRequires: python-setuptools
BuildRequires: openldap-devel

Requires: salt-master >= 2015.5.5
Requires: pytz
Requires: python-cpopen
Requires: python-netaddr
Requires: mongodb
Requires: mongodb-server

%description
SkyRing is a modern, extensible web-based storage management platform
which represents the future of storage management. SkyRing deploys,
manages and monitors various storage technologies and provides
a pluggable framework for tomorrow’s SDS technologies.
SkyRing integrates best-of-breed open source components at its core,
and integrates with the broader management stack.

%prep
%setup -n %{pkg_name}-%{pkg_version}

%build
make build-special
make pybuild

%install
rm -rf $RPM_BUILD_ROOT
install -D skyring $RPM_BUILD_ROOT/usr/bin/skyring
install -D skyring-setup.sh $RPM_BUILD_ROOT/usr/bin/skyring-setup.sh
install -D conf/sample/skyring.conf.sample $RPM_BUILD_ROOT/etc/skyring/skyring.conf
install -D conf/sample/authentication.conf.sample $RPM_BUILD_ROOT/etc/skyring/authentication.conf
install -D conf/skyring_salt_master.conf $RPM_BUILD_ROOT/etc/salt/master.d/skyring.conf
install -m 755 -d $RPM_BUILD_ROOT/srv/salt
install -m 755 -d $RPM_BUILD_ROOT/srv/salt/collectd
install -m 755 -d $RPM_BUILD_ROOT/srv/salt/template
install -m 755 -d $RPM_BUILD_ROOT/srv/salt/collectd/files
install -m 755 -d $RPM_BUILD_ROOT/var/log/skyring
install -D backend/salt/sls/*.* $RPM_BUILD_ROOT/srv/salt/
install -D backend/salt/sls/collectd/*.* $RPM_BUILD_ROOT/srv/salt/collectd
install -D backend/salt/conf/collectd/* $RPM_BUILD_ROOT/srv/salt/collectd/files
install -D backend/salt/template/* $RPM_BUILD_ROOT/srv/salt/template
install -d $RPM_BUILD_ROOT/%{python2_sitelib}/skyring
install -D backend/salt/python/skyring/* $RPM_BUILD_ROOT/%{python2_sitelib}/skyring/

%post

%preun

%clean
rm -rf "$RPM_BUILD_ROOT"

%files
%attr(0755, root, root) /usr/bin/skyring
%attr(0755, root, root) /usr/bin/skyring-setup.sh
%{_sysconfdir}/skyring/*
%{_sysconfdir}/salt/master.d/skyring.conf
%{python2_sitelib}/skyring/*
%{_var}/log/skyring
/srv/salt/*

%changelog
* Thu Dec 03 2015 <tjeyasin@redhat.com>
- Initial build.
