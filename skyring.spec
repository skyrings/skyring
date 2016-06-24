# Determine if systemd will be used
%if ( 0%{?fedora} && 0%{?fedora} > 16 ) || ( 0%{?rhel} && 0%{?rhel} > 6 )
%global with_systemd 1
%endif

%global selinuxtype targeted
%global moduletype  services
%global modulenames skyring

%global _format() export %1=""; for x in %{modulenames}; do %1+=%2; %1+=" "; done;
# Relabel files
%global skyring_relabel_files() %{_sbindir}/restorecon -R %{_bindir}/skyring %{_prefix}/lib/systemd/system/skyring.* /var/lib/skyring/.* /var/run/\.skyring-event /etc/skyring/ /var/log/skyring /etc/httpd &> /dev/null || :
%global salt_relabel_files() %{_sbindir}/restorecon -R %{_bindir}/salt* /etc/salt /var/cache/salt /var/log/salt /var/run/salt /srv/salt /usr/lib/systemd/system/salt* /usr/lib/systemd/system/salt-master /usr/bin/salt-minion /usr/bin/salt-master &> /dev/null || :
%global carbon_relabel_files() %{_sbindir}/restorecon -R %{_bindir}/carbon-aggregator %{_bindir}/carbon-cache %{_bindir}/carbon-client %{_bindir}/carbon-relay %{_bindir}/validate-storage-schemas %{_prefix}/lib/systemd/system/carbon* /var/lib/carbon/ /var/log/carbon/ /var/run/carbon-aggregator.pid /var/run/carbon-cache.pid &> /dev/null || :

# Version of distribution SELinux policy package
%if 0%{?fedora} >= 22
%global selinux_policyver 3.13.1-158.18
%endif
%if 0%{?fedora} >= 25
%global selinux_policyver 3.13.1-188
%endif
%if 0%{?rhel}
%global selinux_policyver 3.13.1-60
%endif

%define pkg_name skyring
%define pkg_version 0.0.30
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
Requires: carbon-selinux
Requires: %{name}-selinux
Requires: salt-selinux
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
Requires: ceph-installer
Requires: mod_ssl

%description
SKYRING is a modern, extensible web-based storage management platform
which represents the future of storage management. SKYRING deploys,
manages and monitors various storage technologies and provides
a plugin framework for tomorrow’s SDS technologies.
SKYRING integrates best-of-breed open source components at its core,
and integrates with the broader management stack.

%package selinux
License: GPLv2
Group: System Environment/Base
Summary: SELinux Policies for Skyring
BuildArch: noarch
Requires(post): selinux-policy-base >= %{selinux_policyver}, selinux-policy-targeted >= %{selinux_policyver}, policycoreutils, policycoreutils-python libselinux-utils
BuildRequires: selinux-policy selinux-policy-devel

%description selinux
SELinux Policies for Skyring


%package -n salt-selinux
License: GPLv2
Group: System Environment/Base
Summary: SELinux Policies for Salt
BuildArch: noarch
Requires(post): selinux-policy-base >= %{selinux_policyver}, selinux-policy-targeted >= %{selinux_policyver}, policycoreutils, policycoreutils-python libselinux-utils
BuildRequires: selinux-policy selinux-policy-devel

%description -n salt-selinux
SELinux Policies for Salt


%package -n carbon-selinux
License: GPLv2
Group: System Environment/Base
Summary: SELinux Policies for Carbon
BuildArch: noarch
Requires(post): selinux-policy-base >= %{selinux_policyver}, selinux-policy-targeted >= %{selinux_policyver}, policycoreutils, policycoreutils-python libselinux-utils
BuildRequires: selinux-policy selinux-policy-devel

%description -n carbon-selinux
SELinux Policies for Carbon

%prep
%setup -n %{pkg_name}-%{pkg_version}

%build
make bzip-selinux-policies
make build-special
make pybuild

%install
rm -rf $RPM_BUILD_ROOT
mkdir -p %{buildroot}%{_sysconfdir}/sysconfig
install -D skyring $RPM_BUILD_ROOT/usr/bin/skyring
install -Dm 0644 conf/sample/graphite-web.conf.sample $RPM_BUILD_ROOT/etc/skyring/httpd/conf.d/graphite-web.conf
install -Dm 0644 conf/sample/skyring-web.conf.sample $RPM_BUILD_ROOT/etc/httpd/conf.d/skyring-web.conf
install -Dm 0644 conf/sample/carbon.conf.sample $RPM_BUILD_ROOT/etc/skyring/carbon/carbon.conf
install -Dm 0644 conf/sample/storage-schemas.conf.sample $RPM_BUILD_ROOT/etc/skyring/carbon/storage-schemas.conf
install -m 755 -d $RPM_BUILD_ROOT/usr/share/skyring/setup
install -Dm 755 skyring-pre.sh $RPM_BUILD_ROOT/usr/bin/skyring-pre.sh
install -Dm 755 skyring-setup.sh $RPM_BUILD_ROOT/usr/share/skyring/setup/skyring-setup.sh
install -Dm 0644 conf/sample/skyring_logrotate.conf.sample $RPM_BUILD_ROOT/etc/logrotate.d/skyring
install -Dm 0644 conf/sample/skyring.conf.sample $RPM_BUILD_ROOT/etc/skyring/skyring.conf
install -Dm 0644 conf/sample/authentication.conf.sample $RPM_BUILD_ROOT/etc/skyring/authentication.conf
install -Dm 0644 conf/skyring_salt_master.conf $RPM_BUILD_ROOT/etc/salt/master.d/skyring.conf
install -Dm 0644 conf/sample/about.conf.sample $RPM_BUILD_ROOT/etc/skyring/about.conf
install -Dm 0644 event/skyring.evt $RPM_BUILD_ROOT/etc/skyring/skyring.evt
install -m 755 -d $RPM_BUILD_ROOT/srv/salt
install -m 755 -d $RPM_BUILD_ROOT/srv/salt/collectd
install -m 755 -d $RPM_BUILD_ROOT/srv/salt/template
install -m 755 -d $RPM_BUILD_ROOT/srv/salt/collectd/files
install -m 755 -d $RPM_BUILD_ROOT/var/log/skyring
install -Dm 0644 backend/salt/sls/*.* $RPM_BUILD_ROOT/srv/salt/
install -Dm 0644 backend/salt/sls/collectd/*.* $RPM_BUILD_ROOT/srv/salt/collectd
install -Dm 0644 backend/salt/conf/collectd/* $RPM_BUILD_ROOT/srv/salt/collectd/files
install -Dm 0755 backend/salt/template/* $RPM_BUILD_ROOT/srv/salt/template
install -d $RPM_BUILD_ROOT/%{python2_sitelib}/skyring
install -D backend/salt/python/skyring/* $RPM_BUILD_ROOT/%{python2_sitelib}/skyring/
install -D -p -m 0644 misc/systemd/%{pkg_name}.service %{buildroot}%{_unitdir}/%{pkg_name}.service
install -p -D -m 644 misc/systemd/%{pkg_name}.sysconf  %{buildroot}%{_sysconfdir}/sysconfig/%{pkg_name}
install -m 755 -d $RPM_BUILD_ROOT/srv/salt/_modules
install -Dm 0644 salt_module/skyring_utils.py $RPM_BUILD_ROOT/srv/salt/_modules
install -Dm 0644 salt_module/collectd.py $RPM_BUILD_ROOT/srv/salt/_modules
gzip skyring.8
install -Dm 0644 skyring.8.gz $RPM_BUILD_ROOT%{_mandir}/man8/skyring.8.gz
chmod -x $RPM_BUILD_ROOT/%{python2_sitelib}/skyring/__init__.py
chmod -x $RPM_BUILD_ROOT/%{python2_sitelib}/skyring/saltwrapper.py
chmod +x $RPM_BUILD_ROOT/srv/salt/push_event.sls

# Install SELinux interfaces and policy modules
install -d %{buildroot}%{_datadir}/selinux/devel/include/%{moduletype}
install -d %{buildroot}%{_datadir}/selinux/packages

# skyring
install -p -m 644 selinux/skyring.if \
	%{buildroot}%{_datadir}/selinux/devel/include/%{moduletype}
install -m 0644 selinux/skyring.pp.bz2 \
	%{buildroot}%{_datadir}/selinux/packages

# salt
install -p -m 644 selinux/salt.if \
	%{buildroot}%{_datadir}/selinux/devel/include/%{moduletype}
install -m 0644 selinux/salt.pp.bz2 \
	%{buildroot}%{_datadir}/selinux/packages

# carbon
install -p -m 644 selinux/carbon.if \
	%{buildroot}%{_datadir}/selinux/devel/include/%{moduletype}
install -m 0644 selinux/carbon.pp.bz2 \
	%{buildroot}%{_datadir}/selinux/packages

%post
ln -fs /usr/share/skyring/setup/skyring-setup.sh /usr/bin/skyring-setup
/bin/systemctl enable skyring.service >/dev/null 2>&1 || :

%post selinux
%_format MODULE %{_datadir}/selinux/packages/skyring.pp.bz2
%{_sbindir}/semodule -n -s %{selinuxtype} -i $MODULE
if %{_sbindir}/selinuxenabled ; then
    %{_sbindir}/load_policy
    %skyring_relabel_files
fi

semanage port -a -t mailbox_port_t -p tcp 2004 2> /dev/null
semanage port -a -t xinuexpansion4_port_t -p tcp 2024 2> /dev/null
semanage port -a -t xinuexpansion3_port_t -p tcp 2023 2> /dev/null
semanage port -a -t skyring_custom_port_t -p tcp 8181 2> /dev/null
semanage boolean -m collectd_tcp_network_connect --on

%post -n salt-selinux
%_format MODULE %{_datadir}/selinux/packages/salt.pp.bz2
%{_sbindir}/semodule -n -s %{selinuxtype} -i $MODULE
if %{_sbindir}/selinuxenabled ; then
    %{_sbindir}/load_policy
    %salt_relabel_files
fi

%post -n carbon-selinux
%_format MODULE %{_datadir}/selinux/packages/carbon.pp.bz2
%{_sbindir}/semodule -n -s %{selinuxtype} -i $MODULE
if %{_sbindir}/selinuxenabled ; then
    %{_sbindir}/load_policy
    %carbon_relabel_files
fi

%postun selinux
if [ $1 -eq 0 ]; then
    %{_sbindir}/semodule -n -r %{modulenames} &> /dev/null || :
    if %{_sbindir}/selinuxenabled ; then
	%{_sbindir}/load_policy
	%skyring_relabel_files
    fi
fi

%postun -n salt-selinux
if [ $1 -eq 0 ]; then
    %{_sbindir}/semodule -n -r %{modulenames} &> /dev/null || :
    if %{_sbindir}/selinuxenabled ; then
	%{_sbindir}/load_policy
	%salt_relabel_files
    fi
fi

%postun -n carbon-selinux
if [ $1 -eq 0 ]; then
    %{_sbindir}/semodule -n -r %{modulenames} &> /dev/null || :
    if %{_sbindir}/selinuxenabled ; then
	%{_sbindir}/load_policy
	%carbon_relabel_files
    fi
fi

%postun
if [ -e /etc/httpd/conf.d/graphite-web.conf.orig -a -h /etc/httpd/conf.d/graphite-web.conf -a ! -e "`readlink /etc/httpd/conf.d/graphite-web.conf`" ] ; then
 mv -f /etc/httpd/conf.d/graphite-web.conf.orig /etc/httpd/conf.d/graphite-web.conf
fi
if [ -e /etc/carbon/carbon.conf.orig -a -h /etc/carbon/carbon.conf -a ! -e "`readlink /etc/carbon/carbon.conf`" ] ; then
 mv -f /etc/carbon/carbon.conf.orig /etc/carbon/carbon.conf
fi
if [ -e /etc/carbon/storage-schemas.conf.orig -a -h /etc/carbon/storage-schemas.conf -a ! -e "`readlink /etc/carbon/storage-schemas.conf`" ] ; then
 mv -f /etc/carbon/storage-schemas.conf.orig /etc/carbon/storage-schemas.conf
fi
rm -f /etc/logrotate.d/skyring


%triggerin -- graphite-web
if [ ! -h /etc/httpd/conf.d/graphite-web.conf -o ! "`readlink /etc/httpd/conf.d/graphite-web.conf`" = "/etc/skyring/httpd/conf.d/graphite-web.conf" ] ; then
  if [ -e /etc/httpd/conf.d/graphite-web.conf ] ; then
    mv -f /etc/httpd/conf.d/graphite-web.conf /etc/httpd/conf.d/graphite-web.conf.orig
  fi
  ln -s /etc/skyring/httpd/conf.d/graphite-web.conf /etc/httpd/conf.d/graphite-web.conf
fi
if [ ! -h /etc/carbon/carbon.conf -o ! "`readlink /etc/carbon/carbon.conf`" = "/etc/skyring/carbon/carbon.conf" ] ; then
  if [ -e /etc/carbon/carbon.conf ] ; then
    mv -f /etc/carbon/carbon.conf /etc/carbon/carbon.conf.orig
  fi
  ln -s /etc/skyring/carbon/carbon.conf /etc/carbon/carbon.conf
fi
if [ ! -h /etc/carbon/storage-schemas.conf -o ! "`readlink /etc/carbon/storage-schemas.conf`" = "/etc/skyring/carbon/storage-schemas.conf" ] ; then
  if [ -e /etc/carbon/storage-schemas.conf ] ; then
    mv -f /etc/carbon/storage-schemas.conf /etc/carbon/storage-schemas.conf.orig
  fi
  ln -s /etc/skyring/carbon/storage-schemas.conf /etc/carbon/storage-schemas.conf
fi

%triggerun -- graphite-web
if [ ! -h /etc/httpd/conf.d/graphite-web.conf -o ! "`readlink /etc/httpd/conf.d/graphite-web.conf`" = "/etc/skyring/httpd/conf.d/graphite-web.conf" ] ; then
  if [ -e /etc/httpd/conf.d/graphite-web.conf ] ; then
    mv -f /etc/httpd/conf.d/graphite-web.conf /etc/httpd/conf.d/graphite-web.conf.orig
  fi
  ln -s /etc/skyring/httpd/conf.d/graphite-web.conf /etc/httpd/conf.d/graphite-web.conf
fi
if [ ! -h /etc/carbon/carbon.conf -o ! "`readlink /etc/carbon/carbon.conf`" = "/etc/skyring/carbon/carbon.conf" ] ; then
  if [ -e /etc/carbon/carbon.conf ] ; then
    mv -f /etc/carbon/carbon.conf /etc/carbon/carbon.conf.orig
  fi
  ln -s /etc/skyring/carbon/carbon.conf /etc/carbon/carbon.conf
fi
if [ ! -h /etc/carbon/storage-schemas.conf -o ! "`readlink /etc/carbon/storage-schemas.conf`" = "/etc/skyring/carbon/storage-schemas.conf" ] ; then
  if [ -e /etc/carbon/storage-schemas.conf ] ; then
    mv -f /etc/carbon/storage-schemas.conf /etc/carbon/storage-schemas.conf.orig
  fi
  ln -s /etc/skyring/carbon/storage-schemas.conf /etc/carbon/storage-schemas.conf
fi

%triggerpostun -- graphite-web
if [ $2 -eq 0 ] ; then
 rm -f /etc/httpd/conf.d/graphite-web.conf.rpmsave /etc/httpd/conf.d/graphite-web.conf.orig
fi
if [ -e /etc/httpd/conf.d/graphite-web.conf.rpmnew ] ; then
 mv /etc/httpd/conf.d/graphite-web.conf.rpmnew /etc/httpd/conf.d/graphite-web.conf.orig
fi
if [ $2 -eq 0 ] ; then
 rm -f /etc/carbon/carbon.conf.rpmsave /etc/carbon/carbon.conf.orig
fi
if [ -e /etc/carbon/carbon.conf.rpmnew ] ; then
 mv /etc/carbon/carbon.conf.rpmnew /etc/carbon/carbon.conf.orig
fi
if [ $2 -eq 0 ] ; then
 rm -f /etc/carbon/storage-schemas.conf.rpmsave /etc/carbon/storage-schemas.conf.orig
fi
if [ -e /etc/carbon/storage-schemas.conf.rpmnew ] ; then
 mv /etc/carbon/storage-schemas.conf.rpmnew /etc/carbon/storage-schemas.conf.orig
fi

%clean
rm -rf "$RPM_BUILD_ROOT"

%files selinux
%defattr(-,root,root,0755)
%attr(0644,root,root) %{_datadir}/selinux/packages/skyring.pp.bz2
%attr(0644,root,root) %{_datadir}/selinux/devel/include/%{moduletype}/skyring.if

%files -n salt-selinux
%defattr(-,root,root,0755)
%attr(0644,root,root) %{_datadir}/selinux/packages/salt.pp.bz2
%attr(0644,root,root) %{_datadir}/selinux/devel/include/%{moduletype}/salt.if

%files -n carbon-selinux
%defattr(-,root,root,0755)
%attr(0644,root,root) %{_datadir}/selinux/packages/carbon.pp.bz2
%attr(0644,root,root) %{_datadir}/selinux/devel/include/%{moduletype}/carbon.if

%files
%attr(0755, root, root) /usr/bin/skyring
%attr(0755, root, root) /usr/bin/skyring-pre.sh
%attr(0755, root, root) /usr/share/skyring/setup/skyring-setup.sh
%{python2_sitelib}/skyring/*
%{_var}/log/skyring
/srv/salt/*
/srv/salt/_modules/*
%{_unitdir}/%{pkg_name}.service
%config(noreplace) %{_sysconfdir}/sysconfig/%{pkg_name}
%config(noreplace) %attr(644,root,root) %{_sysconfdir}/skyring/httpd/conf.d/graphite-web.conf
%config(noreplace) %attr(644,root,root) %{_sysconfdir}/httpd/conf.d/skyring-web.conf
%config(noreplace) %attr(644,root,root) %{_sysconfdir}/skyring/carbon/carbon.conf
%config(noreplace) %attr(644,root,root) %{_sysconfdir}/skyring/carbon/storage-schemas.conf
%config(noreplace) %{_sysconfdir}/skyring/authentication.conf
%config(noreplace) %{_sysconfdir}/skyring/skyring.conf
%config(noreplace) %{_sysconfdir}/salt/master.d/skyring.conf
%config(noreplace) %{_sysconfdir}/skyring/about.conf
%config(noreplace) %{_sysconfdir}/skyring/skyring.evt
%config(noreplace) %{_sysconfdir}/logrotate.d/skyring
%doc README.md
%{_mandir}/man8/skyring.8.gz

%changelog
* Tue Dec 29 2015 <shtripat@redhat.com> 0.0.1-1
- Added daemonizing mechanism

* Thu Dec 03 2015 Timothy Asir Jeyasingh <tjeyasin@redhat.com> 0.0.1
- Initial build.
