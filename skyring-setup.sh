#!/bin/bash
# Configure and setting up the system to use skyring

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'
CWD=`pwd`

function exit_on_error {
    echo "Error: $1"
    exit -1
}

function create_and_install_certificate {
    echo "Creating self-signed TLS certificate"
    mkdir -p ~/.skyring
    cd ~/.skyring
    openssl genrsa -des3 -out skyring.key 1024 || exit_on_error "Failed to generate SSL key"
    openssl req -new -key skyring.key -out skyring.csr || exit_on_error "Failed to create SSL certificate"
    cp skyring.key skyring.key.org
    openssl rsa -in skyring.key.org -out skyring.key || exit_on_error "Failed to create SSL certificate"
    openssl x509 -req -days 365 -in skyring.csr -signkey skyring.key -out skyring.crt || exit_on_error "Failed to sign the SSL certificate"
    cp skyring.key /etc/pki/tls
    cp skyring.crt /etc/pki/tls
    cd -
    \rm -rf ~/.skyring
}

function info {
    printf "${GREEN}$1${NC}\n"
}

if [[ $EUID -ne 0 ]]; then
    echo "This script must be run as root" 1>&2
    exit 1
fi

info "Configure and setting up the system to use skyring"

info "Starting services"
# Enable and start the salt-master:
systemctl enable salt-master
systemctl start salt-master

# Enable and start MongoDB
systemctl enable mongod
systemctl start mongod

info "Setup graphite user"
/usr/lib/python2.7/site-packages/graphite/manage.py syncdb

chown apache:apache /var/lib/graphite-web/graphite.db
systemctl start carbon-cache && systemctl enable carbon-cache
systemctl start httpd

echo "Skyring uses HTTPS to secure the web interface."
read -p "Do you wish to generate and use a self-signed certificate? (Y/N): " yn
case $yn in
	[Yy]* )
		create_and_install_certificate
		;;
	[Nn]* )
		;;
esac

grep -q "sslEnabled" /etc/skyring/skyring.conf || sed -i -e 's/"config".*{/"config": {\n\t"sslEnabled": true,/g' /etc/skyring/skyring.conf

read -p "Please enter the FQDN of server [`hostname`]:" hostname

if [ -z $hostname ]
then
    hostname=`hostname`
fi

echo 'Define host_name' $hostname | cat - /etc/httpd/conf.d/skyring-web.conf > temp && mv -f temp /etc/httpd/conf.d/skyring-web.conf

# Start the skyring server
systemctl enable skyring
systemctl start skyring

systemctl restart httpd && systemctl enable httpd

info "\n\n\n-------------------------------------------------------"
info "Now the skyring setup is ready!"
info "You can start/stop/restart the server by executing the command"
info "\tsystemctl start/stop/restart skyring"
info "Skyring log directory: /var/log/skyring"
info "Mongodb user name: admin"
info "Mongodb password: admin"
info "URLs to access skyring services"
info "  - http://${hostname}/skyring"
info "  - http://${hostname}/graphite-web"
info "-------------------------------------------------------"

info "Done!"
