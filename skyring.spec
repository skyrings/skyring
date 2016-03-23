# Determine if systemd will be used
%if ( 0%{?fedora} && 0%{?fedora} > 16 ) || ( 0%{?rhel} && 0%{?rhel} > 6 )
%global with_systemd 1
%endif

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

%if 0%{?with_systemd}
BuildRequires:  systemd
Requires(post): systemd
Requires(preun): systemd
Requires(postun): systemd
%else
Requires(post):   /sbin/chkconfig
Requires(preun):  /sbin/service
Requires(preun):  /sbin/chkconfig
Requires(postun): /sbin/service
%endif

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
Requires: graphite-web
Requires: python-carbon
Requires: python-whisper
Requires: redhat-ceph-installer

%description
skyring is a modern, extensible web-based storage management platform
which represents the future of storage management. skyring deploys,
manages and monitors various storage technologies and provides
a pluggable framework for tomorrow’s SDS technologies.
skyring integrates best-of-breed open source components at its core,
and integrates with the broader management stack.

%prep
%setup -n %{pkg_name}-%{pkg_version}

%build
make build-special
make pybuild

%install
rm -rf $RPM_BUILD_ROOT
install -D skyring $RPM_BUILD_ROOT/usr/bin/skyring
install -D conf/sample/graphite-web.conf.sample $RPM_BUILD_ROOT/etc/skyring/httpd/conf.d/graphite-web.conf
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
install -D -p -m 0644 misc/systemd/%{name}d.service %{buildroot}%{_unitdir}/%{name}d.service
install -D -p -m 0755 misc/etc.init/%{name}.initd %{buildroot}%{_sysconfdir}/init.d/%{name}d

%post

%preun

%postun
if [ -e /etc/httpd/conf.d/graphite-web.conf.orig -a -h /etc/httpd/conf.d/graphite-web.conf -a ! -e "`readlink /etc/httpd/conf.d/graphite-web.conf`" ] ; then
 mv -f /etc/httpd/conf.d/graphite-web.conf.orig /etc/httpd/conf.d/graphite-web.conf
fi

%triggerin -- graphite-web
if [ ! -h $RPM_BUILD_ROOT/etc/httpd/conf.d/graphite-web.conf -o ! "`readlink $RPM_BUILD_ROOT/etc/httpd/conf.d/graphite-web.conf`" = "/etc/skyring/httpd/conf.d/graphite-web.conf" ] ; then
  if [ -e $RPM_BUILD_ROOT/etc/httpd/conf.d/graphite-web.conf ] ; then
    mv -f $RPM_BUILD_ROOT/etc/httpd/conf.d/graphite-web.conf $RPM_BUILD_ROOT/etc/httpd/conf.d/graphite-web.conf.orig
  fi
  ln -s /etc/skyring/httpd/conf.d/graphite-web.conf $RPM_BUILD_ROOT/etc/httpd/conf.d/graphite-web.conf
fi

%triggerun -- graphite-web
if [ ! -h $RPM_BUILD_ROOT/etc/httpd/conf.d/graphite-web.conf -o ! "`readlink $RPM_BUILD_ROOT/etc/httpd/conf.d/graphite-web.conf`" = "/etc/skyring/httpd/conf.d/graphite-web.conf" ] ; then
  if [ -e $RPM_BUILD_ROOT/etc/httpd/conf.d/graphite-web.conf ] ; then
    mv -f $RPM_BUILD_ROOT/etc/httpd/conf.d/graphite-web.conf $RPM_BUILD_ROOT/etc/httpd/conf.d/graphite-web.conf.orig
  fi
  ln -s /etc/skyring/httpd/conf.d/graphite-web.conf /etc/httpd/conf.d/graphite-web.conf
fi

%triggerpostun -- graphite-web
if [ $2 -eq 0 ] ; then
 rm -f /etc/httpd/conf.d/graphite-web.conf.rpmsave /etc/httpd/conf.d/graphite-web.conf.orig
fi
if [ -e /etc/httpd/conf.d/graphite-web.conf.rpmnew ] ; then
 mv /etc/httpd/conf.d/graphite-web.conf.rpmnew /etc/httpd/conf.d/graphite-web.conf.orig
fi

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
%{_unitdir}/%{name}d.service
%{_sysconfdir}/init.d/%{name}d
%config(noreplace) %attr(644,root,root) %{_sysconfdir}/skyring/httpd/conf.d/graphite-web.conf



%changelog
* Tue Dec 29 2015 <shtripat@redhat.com>
- Added daemonizing mechanism

* Thu Dec 03 2015 <tjeyasin@redhat.com>
- Initial build.
