#!/bin/bash
# This script is tested in fedora 22 server
# This is a single script which will install required packages,
# Configure and setting up the system to use skyring

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'
CWD=`pwd`

function info {
    printf "${GREEN}$1${NC}\n"
}

function createUser {
    /opt/influxdb/influx  -execute "CREATE USER admin WITH PASSWORD 'admin'"
    # Grant the privileges
    /opt/influxdb/influx  -execute "GRANT ALL ON collectdInfluxDB TO admin"
}

if [[ $EUID -ne 0 ]]; then
    echo "This script must be run as root" 1>&2
    exit 1
fi

info "Configure and setting up the system to use skyring"
info "Disabling firewalld"
systemctl stop firewalld && systemctl disable firewalld

info "Starting services"
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
info "You can start the server by executing 'skyring'"
info "Skyring log directory: /var/log/skyring"
info "Influxdb user name: admin"
info "Influxdb password: admin"
info "Mongodb user name: admin"
info "Mongodb password: admin"
info "-------------------------------------------------------"
info "Done!"
