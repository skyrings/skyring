# Determine if systemd will be used
%if ( 0%{?fedora} && 0%{?fedora} > 16 ) || ( 0%{?rhel} && 0%{?rhel} > 6 )
%global with_systemd 1
%endif

%define pkg_name skyring
%define pkg_version 0.0.9
%define pkg_release 1

Name: %{pkg_name}
Version: %{pkg_version}
Release: %{pkg_release}%{?dist}
Summary: Modern extensible web-based storage management platform
Source0: %{pkg_name}-%{pkg_version}.tar.gz
License: ASL 2.0
Group: Applications/System
BuildRoot: %{_tmppath}/%{pkg_name}-%{pkg_version}-%{pkg_release}-buildroot
Url: http://github.com/skyrings/skyring

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
%if ( 0%{?fedora} && 0%{?fedora} > 16 )
Requires: mongodb-org
Requires: mongodb-org-server
%else
Requires: mongodb
Requires: mongodb-server
%endif
Requires: graphite-web
Requires: python-carbon
Requires: python-whisper
Requires: redhat-ceph-installer

%description
SKYRING is a modern, extensible web-based storage management platform
which represents the future of storage management. SKYRING deploys,
manages and monitors various storage technologies and provides
a plugin framework for tomorrow’s SDS technologies.
SKYRING integrates best-of-breed open source components at its core,
and integrates with the broader management stack.

%prep
%setup -n %{pkg_name}-%{pkg_version}

%build
make build-special
make pybuild

%install
rm -rf $RPM_BUILD_ROOT
install -D skyring $RPM_BUILD_ROOT/usr/bin/skyring
install -Dm 0644 conf/sample/graphite-web.conf.sample $RPM_BUILD_ROOT/etc/skyring/httpd/conf.d/graphite-web.conf
install -m 755 -d $RPM_BUILD_ROOT/usr/share/skyring/setup
install -Dm 755 skyring-setup.sh $RPM_BUILD_ROOT/usr/share/skyring/setup/skyring-setup.sh
install -Dm 0644 conf/sample/skyring.conf.sample $RPM_BUILD_ROOT/etc/skyring/skyring.conf
install -Dm 0644 conf/sample/authentication.conf.sample $RPM_BUILD_ROOT/etc/skyring/authentication.conf
install -Dm 0644 conf/skyring_salt_master.conf $RPM_BUILD_ROOT/etc/salt/master.d/skyring.conf
install -Dm 0644 conf/sample/about.conf.sample $RPM_BUILD_ROOT/etc/skyring/about.conf
install -m 755 -d $RPM_BUILD_ROOT/srv/salt
install -m 755 -d $RPM_BUILD_ROOT/srv/salt/collectd
install -m 755 -d $RPM_BUILD_ROOT/srv/salt/template
install -m 755 -d $RPM_BUILD_ROOT/srv/salt/collectd/files
install -m 755 -d $RPM_BUILD_ROOT/var/log/skyring
install -Dm 0755 backend/salt/sls/*.* $RPM_BUILD_ROOT/srv/salt/
install -Dm 0644 backend/salt/sls/collectd/*.* $RPM_BUILD_ROOT/srv/salt/collectd
install -Dm 0644 backend/salt/conf/collectd/* $RPM_BUILD_ROOT/srv/salt/collectd/files
install -Dm 0755 backend/salt/template/* $RPM_BUILD_ROOT/srv/salt/template
install -d $RPM_BUILD_ROOT/%{python2_sitelib}/skyring
install -D backend/salt/python/skyring/* $RPM_BUILD_ROOT/%{python2_sitelib}/skyring/
install -D -p -m 0644 misc/systemd/%{name}d.service %{buildroot}%{_unitdir}/%{name}d.service
install -D -p -m 0755 misc/etc.init/%{name}.initd %{buildroot}%{_sysconfdir}/init.d/%{name}d
gzip skyring.8
install -Dm 0644 skyring.8.gz $RPM_BUILD_ROOT%{_mandir}/man8/skyring.8.gz

%post
ln -fs %{buildroot}/usr/share/skyring/setup/skyring-setup.sh %{buildroot}/usr/bin/skyring-setup
/sbin/chkconfig --add skyringd
/sbin/chkconfig --level 35 skyringd on

%preun
/sbin/chkconfig --level 35 skyringd off
if [ "$1" = 0 ] ; then
    /sbin/service skyringd stop > /dev/null 2>&1
    /sbin/chkconfig --del skyringd
fi

%postun
if [ -e /etc/httpd/conf.d/graphite-web.conf.orig -a -h /etc/httpd/conf.d/graphite-web.conf -a ! -e "`readlink /etc/httpd/conf.d/graphite-web.conf`" ] ; then
 mv -f /etc/httpd/conf.d/graphite-web.conf.orig /etc/httpd/conf.d/graphite-web.conf
fi

%triggerin -- graphite-web
if [ ! -h %{buildroot}/etc/httpd/conf.d/graphite-web.conf -o ! "`readlink %{buildroot}/etc/httpd/conf.d/graphite-web.conf`" = "/etc/skyring/httpd/conf.d/graphite-web.conf" ] ; then
  if [ -e %{buildroot}/etc/httpd/conf.d/graphite-web.conf ] ; then
    mv -f %{buildroot}/etc/httpd/conf.d/graphite-web.conf %{buildroot}/etc/httpd/conf.d/graphite-web.conf.orig
  fi
  ln -s /etc/skyring/httpd/conf.d/graphite-web.conf %{buildroot}/etc/httpd/conf.d/graphite-web.conf
fi

%triggerun -- graphite-web
if [ ! -h %{buildroot}/etc/httpd/conf.d/graphite-web.conf -o ! "`readlink %{buildroot}/etc/httpd/conf.d/graphite-web.conf`" = "/etc/skyring/httpd/conf.d/graphite-web.conf" ] ; then
  if [ -e %{buildroot}/etc/httpd/conf.d/graphite-web.conf ] ; then
    mv -f %{buildroot}/etc/httpd/conf.d/graphite-web.conf %{buildroot}/etc/httpd/conf.d/graphite-web.conf.orig
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
%attr(0755, root, root) /usr/share/skyring/setup/skyring-setup.sh
%{python2_sitelib}/skyring/*
%{_var}/log/skyring
/srv/salt/*
%{_unitdir}/%{name}d.service
%{_sysconfdir}/init.d/%{name}d
%config(noreplace) %attr(644,root,root) %{_sysconfdir}/skyring/httpd/conf.d/graphite-web.conf
%config(noreplace) %{_sysconfdir}/skyring/authentication.conf
%config(noreplace) %{_sysconfdir}/skyring/skyring.conf
%config(noreplace) %{_sysconfdir}/salt/master.d/skyring.conf
%config(noreplace) %{_sysconfdir}/skyring/about.conf
%doc README.md

%changelog
* Tue Dec 29 2015 <shtripat@redhat.com> 0.0.1-1
- Added daemonizing mechanism

* Thu Dec 03 2015 Timothy Asir Jeyasingh <tjeyasin@redhat.com> 0.0.1
- Initial build.
