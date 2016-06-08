#!/bin/bash
# Configure and setting up the system to use skyring

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'
CWD=`pwd`

function info {
    printf "${GREEN}$1${NC}\n"
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

# Enable and start MongoDB
systemctl enable mongod
systemctl start mongod

# Need to wait for 3 to 5 sec for the services to comes up
info "Setting up mongodb...."
sleep 10

mongo <<EOF
use skyring
db.leads.findOne()
show collections
db.createUser( { "user" : "admin", "pwd": "admin", "roles" : ["readWrite", "dbAdmin", "userAdmin"] })
show users
EOF

# Configuring logrotation
cat <<EOF >/etc/logrotate.d/skyring
/var/log/skyring/skyring.log {
    su root root
    size=100M
    rotate 10
    missingok
    compress
    notifempty
    create 0664 root root
}
EOF

cat <<EOF >/etc/logrotate.d/skyring
/var/log/skyring/bigfin.log {
    su root root
    size=100M
    rotate 10
    missingok
    compress
    notifempty
    create 0664 root root
}
EOF


# Start the skyring server
systemctl enable skyringd
systemctl start skyringd

info "Setup graphite user"
/usr/lib/python2.7/site-packages/graphite/manage.py syncdb

chown apache:apache /var/lib/graphite-web/graphite.db
service carbon-cache start && chkconfig carbon-cache on
service httpd start && chkconfig httpd on

info "\n\n\n-------------------------------------------------------"
info "Now the skyring setup is ready!"
info "You can start/stop/restart the server by executing the command"
info "\tsystemctl start/stop/restart skyringd"
info "Skyring log directory: /var/log/skyring"
info "Mongodb user name: admin"
info "Mongodb password: admin"
info "-------------------------------------------------------"

info "Done!"
