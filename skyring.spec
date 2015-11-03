%define name skyring
%define version 1.0
%define release 1

Name: %{name}
Version: %{version}
#Release: %{release}
Release: 1%{?dist}
Summary: Modern extensible web-based storage management platform
Source0: %{name}-%{version}.tar.gz
License: Apache-2.0
Group: Applications/Storage
#BuildRoot: %{_tmppath}/%{name}-%{version}-%{release}
BuildRoot: %{_tmppath}/%{name}-%{version}-%{release}-buildroot
Vendor: Red Hat, Inc. <skyring@redhat.com>
Url: github.com/skyrings/skyring

%description
SkyRing is a modern, extensible web-based storage management platform
which represents the future of storage management. SkyRing deploys,
manages and monitors various storage technologies and provides
a pluggable framework for tomorrow’s SDS technologies.
SkyRing integrates best-of-breed open source components at its core,
and integrates with the broader management stack.

%prep
%setup -n %{name}-%{version}

%build
python setup.py build

%install
rm -rf $RPM_BUILD_ROOT
install -D _skyring $RPM_BUILD_ROOT/usr/bin/skyring
install -D conf/sample/skyring.conf.sample $RPM_BUILD_ROOT/etc/skyring/skyring.conf
install -D conf/sample/authentication.conf.sample $RPM_BUILD_ROOT/etc/skyring/authentication.conf
install -D conf/sample/providers.d/ceph.conf.sample $RPM_BUILD_ROOT/etc/skyring/providers.d/ceph.conf
install -m 755 -d $RPM_BUILD_ROOT/srv/salt
install -D backend/salt/sls/* $RPM_BUILD_ROOT/srv/salt/
install -d $RPM_BUILD_ROOT/usr/lib/python2.7/site-packages/skyring
install -D backend/salt/python/skyring/* $RPM_BUILD_ROOT/usr/lib/python2.7/site-packages/skyring/

%post
if grep -q "reactor:" "/etc/salt/master"; then
    info "Salt master already configured!"
else
cat >> /etc/salt/master <<EOF
reactor:
 - 'salt/minion/*/start':
  - /srv/salt/push_event.sls
EOF
fi

# Disabling firewalld
systemctl stop firewalld
systemctl disable firewalld

# Enable and start the salt-master:
systemctl enable salt-master
systemctl start salt-master

#Enable and start InfluxDB:
systemctl enable influxdb
systemctl start influxdb

# Enable and start MongoDB
systemctl enable mongod
systemctl start mongod

# Need to wait for 3 to 5 sec for the services to comes up
sleep 5
info "Creating time series database"
# Create influxdb Database
/opt/influxdb/influx  -execute "CREATE DATABASE IF NOT EXISTS collectd"

# Create InfluxDB User
/opt/influxdb/influx  -execute 'SHOW USERS' -format column | awk '{print $1}' | grep -Fxq 'admin' || createUser

info "Creating skyring database"
# Configuring MongoDB
mongo <<EOF
use skyring
db.leads.findOne()
show collections
db.createUser( { "user" : "admin", "pwd": "admin", "roles" : ["readWrite", "dbAdmin", "userAdmin"] })
show users
EOF

/usr/bin/skyring install 1> /dev/null

%preun
/usr/bin/skyring uninstall --package 1> /dev/null

%clean
rm -rf "$RPM_BUILD_ROOT"

%files
#-f INSTALLED_FILES
%exclude /usr/lib/python2.7/site-packages/skyring-0.0.1-py2.7.egg-info/*
%attr(0755, root, root) /usr/bin/skyring
/etc/skyring/*
/etc/skyring/providers.d/*
/srv/salt/*
/usr/lib/python2.7/site-packages/skyring/*
