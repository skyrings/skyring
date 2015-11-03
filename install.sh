#!/bin/bash
# Configure and setting up the system to use skyring

echo "Configure and setting up the system to use skyring"
echo "Disabling firewalld"
systemctl stop firewalld
systemctl disable firewalld

echo "Starting services"
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

echo "Creating time series database"
# Create influxdb Database
/opt/influxdb/influx  -execute "CREATE DATABASE IF NOT EXISTS collectd"

# Create InfluxDB User
/opt/influxdb/influx  -execute 'SHOW USERS' -format column | awk '{print $1}' | grep -Fxq 'admin' || createUser

echo "Creating skyring database"
# Configuring MongoDB
mongo <<EOF
use skyring
db.leads.findOne()
show collections
db.createUser( { "user" : "admin", "pwd": "admin", "roles" : ["readWrite", "dbAdmin", "userAdmin"] })
show users
EOF
echo "Done!"
