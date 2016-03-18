#!/bin/bash
# This script is tested in fedora 22 server
# This is a single script which will install required packages,
# Configure and setting up the system to use skyring

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'
CWD=`pwd`

function error {
    printf "${RED}$1${NC}\n"
}

function info {
    printf "${NC}$1${NC}\n"
}

function warning {
    printf "${YELLOW}$1${NC}\n"
}

function debug {
    printf "${GREEN}$1${NC}\n"
}

function createUser {
    /opt/influxdb/influx  -execute "CREATE USER admin WITH PASSWORD 'admin'"
    # Grant the privileges
    /opt/influxdb/influx  -execute "GRANT ALL ON collectdInfluxDB TO admin"
}

# Check for the environment
if ! grep -q '^Fedora release 22 (Twenty Two)$' /etc/issue; then
    error "Currently this script support only fedora 22 server"
   exit 1
fi

if [[ $EUID -ne 0 ]]; then
    echo "This script must be run as root" 1>&2
    exit 1
fi


# Package installation
info "Installing necessary packages for skyring\n"
set -x
yum -y update
if [ $? -ne 0 ]; then
    error "Failed to update packages"
fi

yum -y install git golang salt-master python python-devel python-cpopen python-paramiko python-netaddr hg mongodb mongodb-server openldap-devel ceph http://influxdb.s3.amazonaws.com/influxdb-0.9.4.2-1.x86_64.rpm
if [ $? -ne 0 ]; then
    error "Package installation failed"
    exit 1
fi

set +x
export GOROOT=/usr/lib/golang/
export GOPATH=~/.skyring_build/golang/gopath
export PATH=$PATH:$GOPATH/bin:$GOROOT/bin

info "Creating go build path and structure"
mkdir ~/.skyring_build/golang/gopath/src/github.com/skyrings/ -p
mkdir -p /var/lib/skyring/providers
mkdir /srv/salt -p
mkdir -p /etc/skyring/providers.d
mkdir -p /var/log/skyring
chmod 777 /var/log/skyring

# Build packages
cd ~/.skyring_build/golang/gopath/src/github.com/skyrings/
[ -d skyring ] || git clone https://review.gerrithub.io/skyrings/skyring
cd skyring
make

cd ~/.skyring_build/golang/gopath/src/github.com/skyrings/
[ -d bigfin ] || git clone https://review.gerrithub.io/skyrings/bigfin
cd bigfin
make
cd $CWD

# Configuring packages
# ~~~~~~~~~~~~~~~~~~~~

info "Configuring Skyring providers\n"
# Copy the salt template files:
cp -rf ~/.skyring_build/golang/gopath/src/github.com/skyrings/skyring/backend/salt/sls/* /srv/salt/
mkdir -p /srv/salt/collectd/files
cp -rf ~/.skyring_build/golang/gopath/src/github.com/skyrings/skyring/backend/salt/conf/collectd/* /srv/salt/collectd/files
cp -rf ~/.skyring_build/golang/gopath/src/github.com/skyrings/bigfin/backend/salt/sls/* /srv/salt/

# Create the configuration files
cp ~/.skyring_build/golang/gopath/src/github.com/skyrings/skyring/conf/sample/skyring.conf.sample /etc/skyring/skyring.conf
cp ~/.skyring_build/golang/gopath/src/github.com/skyrings/skyring/conf/sample/authentication.conf.sample /etc/skyring/authentication.conf
cp ~/.skyring_build/golang/gopath/src/github.com/skyrings/skyring/conf/sample/providers.d/ceph.conf.sample /etc/skyring/providers.d/ceph.conf
cp ~/.skyring_build/golang/gopath/src/github.com/skyrings/skyring/conf/sample/about.conf.sample /etc/skyring/about.conf
cp ~/.skyring_build/golang/gopath/src/github.com/skyrings/skyring/conf/sample/providers.d/ceph.dat.sample /etc/skyring/providers.d/ceph.dat

# Copy the binaries into appropriate path
cp -f ~/.skyring_build/golang/gopath/bin/skyring /usr/sbin/.
cp -f ~/.skyring_build/golang/gopath/src/github.com/skyrings/bigfin/ceph_provider /var/lib/skyring/providers

# Configuring host ip into skyring.conf
hostip=`ip route get 1 | awk '{print $NF;exit}'`
sed -i -e "s/\"host\": \"127.0.0.1\"/\"host\": \"${hostip}\"/g" /etc/skyring/skyring.conf

# Build python modules
info "Building python modules"
mkdir -p ~/.skyring_build/ceph-provider-modules
cd ~/.skyring_build/golang/gopath/src/github.com/skyrings/bigfin/backend/salt/python/
python setup.py install --root ~/.skyring_build/ceph-provider-modules
cp -r ~/.skyring_build/ceph-provider-modules/usr/lib/python2.7/site-packages/skyring /usr/lib/python2.7/site-packages/

mkdir -p ~/.skyring_build/skyring-provider-modules
cd ~/.skyring_build/golang/gopath/src/github.com/skyrings/skyring/backend/salt/python
python setup.py install --root ~/.skyring_build/skyring-provider-modules
cp -r ~/.skyring_build/skyring-provider-modules/usr/lib/python2.7/site-packages/skyring /usr/lib/python2.7/site-packages/

rm -fr /tmp/.skyring-event
rm -fr /root/.local/lib/python2.7/site-packages/skyring*.egg


debug "Configuring salt master"
if grep -q "reactor:" "/etc/salt/master"; then
    info "Salt master already configured!"
else
cat >> /etc/salt/master <<EOF

reactor:
  - 'salt/minion/*/start':
    - /srv/salt/push_event.sls

  - 'skyring/*':
    - /srv/salt/push_event.sls

  - 'salt/presence/change':
    - /srv/salt/push_event.sls

presence_events: True
EOF
fi

export PATH=$PATH:/opt/influxdb

info "Starting services"
# Start services
# ~~~~~~~~~~~~~~~

# Disabling firewalld
systemctl stop firewalld && systemctl disable firewalld

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

info "\n\n\n-------------------------------------------------------"
info "Now the host setup is ready!"
info "You can start the server by executing the following cmd:"
info "PYTHONPATH=~/.local/lib/python2.7/site-packages skyring"
info "Skyring log directory: /var/log/skyring"
info "Influxdb user name: admin"
info "Influxdb password: admin"
info "Mongodb user name: admin"
info "Mongodb password: admin"
info "-------------------------------------------------------"
