#!/bin/bash
# This script is tested in fedora 22 server
# This is a single script which will install required packages,
# Configure and setting up the system to use skyring

set -e

# Package installation
GREEN='\033[0;32m'
NC='\033[0m'

DIST=`tr -s ' \011' '\012' < /etc/issue | head -n 1`
if [ ! "$DIST" == "Fedora" ]; then
    printf "${GREEN}Currently this script support only fedora 22 server\n${NC}\n"
    exit
fi

printf "${GREEN}Installing necessary packages for skyring\n${NC}\n"

set -x
yum -y install git golang salt-master
yum -y install python python-devel python-cpopen python-paramiko hg 
yum -y install mongodb mongodb-server openldap-devel

set +x
# Install and configure InfluxDB
if rpm -qa | grep influxdb-0.9  2>&1 > /dev/null; then
    printf "${GREEN}Influxdb already installed${NC}\n"
    set -x
else
    printf "${GREEN}Downloading influxdb package and installing${NC}\n"
    set -x
    wget http://influxdb.s3.amazonaws.com/influxdb-0.9.4.2-1.x86_64.rpm
    yum -y install influxdb-0.9.4.2-1.x86_64.rpm

    #Enable and start InfluxDB:
    systemctl enable influxdb
    systemctl start influxdb
fi

set +x
INSDIR=`pwd`
[ -d skyring ] || git clone https://review.gerrithub.io/skyrings/skyring

printf "${GREEN}Creating go build path and structure\n${NC}"
mkdir ${HOME}/golang/gopath/src/github.com/skyrings/ -p
ln -sf ${INSDIR}/skyring ${HOME}/golang/gopath/src/github.com/skyrings/.

while true; do
    read -p "Do you wish to configure go required path and update your bashrc file [Y/N]:" yn
    case $yn in
	[YyNn]* ) break;;
	* ) echo "Please answer Y or N.";;
    esac
done

if [ $yn = "Y" -o $yn = "y" ]
then
    printf "${GREEN}Configuring go path .${NC}"
    PATH1='$PATH'
    grep -q -F 'export GOROOT=/usr/lib/golang/' ~/.bashrc || echo 'export GOROOT=/usr/lib/golang/' >> ~/.bashrc
    printf "."
    sleep 1
    grep -q -F "export GOPATH=$HOME/golang/gopath" ~/.bashrc || echo "export GOPATH=$HOME/golang/gopath" >> ~/.bashrc
    printf "."
    sleep 1
    grep -q -F "export PATH=\$PATH:\$GOPATH/bin:\$GOROOT/bin" ~/.bashrc || echo "export PATH=\$PATH:\$GOPATH/bin:\$GOROOT/bin" >> ~/.bashrc
    printf ".\n"
    source ~/.bashrc
fi

cd  ${HOME}/golang/gopath/src/github.com/skyrings/skyring
make

# Disabling firewalld
systemctl stop firewalld
systemctl disable firewalld

set +x

# Configuring salt-master
printf "${GREEN}Configuring salt master${NC}\n"
if grep -q "reactor:" "/etc/salt/master"; then
    echo "Salt master already configured!"
else
    echo "" >> /etc/salt/master
    echo "reactor:" >> /etc/salt/master
    echo "- 'salt/minion/*/start':" >> /etc/salt/master
    echo "- /srv/salt/push_event.sls" >> /etc/salt/master
fi


# Enable and start the salt-master:
systemctl enable salt-master
systemctl start salt-master

export PATH=$PATH:/opt/influxdb

set +x
printf "${GREEN}Creating admin user for influxdb database management${NC}\n"
read -p "Influxdb Admin User Name [admin]:" influxuser
influxuser=${influxuser:-admin}
read -sp "Influxdb Admin Password [admin]:" influxpwd
influxpwd=${influxpwd:-admin}
set -x

# Create influxdb Database
/opt/influxdb/influx  -execute "CREATE DATABASE collectd"  || true
# Create InfluxDB User
/opt/influxdb/influx  -execute "CREATE USER ${influxuser} WITH PASSWORD '${influxpwd}'" || true
# Grant the privileges
/opt/influxdb/influx  -execute "GRANT ALL ON collectdInfluxDB TO ${influxuser}" || true

mongouser="admin"
mongopwd="admin"
set +x
printf "${GREEN}Creating admin user in mongo db${NC}\n"
read -p "Mongo Admin User Name [admin]:" mongouser
mongouser=${mongouser:-admin}
read -sp "Mongo Admin Password [admin]:" mongopwd
mongopwd=${mongopwd:-admin}
set -x

# Enable and start MongoDB
systemctl enable mongod
systemctl start mongod

# Configuring MongoDB
function configureMongoScript {
    mongo <<EOF
    use skyring
    db.leads.findOne()
    show collections
    db.createUser( { "user" : "${mongouser}", "pwd": "${mongopwd}", "roles" : ["readWrite", "dbAdmin", "userAdmin"] })
    show users
EOF
}

configureMongoScript

set +x
printf "${GREEN}Configuring Skyring providers${NC}\n"
set -x
cd $HOME/golang/gopath/src/github.com/skyrings/skyring

# Copy the salt template files:
mkdir /srv/salt -p
cp -f salt/* /srv/salt/

# Create the configuration directory and copy the config files
cd $HOME/golang/gopath/src/github.com/skyrings/skyring/conf/sample/
mkdir -p /etc/skyring
mkdir -p /etc/skyring/providers.d
cp skyring.conf.sample /etc/skyring/skyring.conf
cp authentication.conf.sample /etc/skyring/authentication.conf
cp providers.d/ceph.conf.sample /etc/skyring/providers.d/ceph.conf
cp providers.d/gluster.conf.sample /etc/skyring/providers.d/gluster.conf 

set +x
# Configuring host ip into skyring.conf
ip=`ip route get 1 | awk '{print $NF;exit}'`
printf "${GREEN}Enter the local host ip to update skyring.conf${NC}\n"
read -p "Local Server IP Addr [$ip]:" hostip
hostip=${hostip:-$ip}
sed -i -e "s/127.0.0.1/${hostip}/g" /etc/skyring/skyring.conf

# Configuring skyring log
printf "${GREEN}Configuring skyring log${NC}\n"
printf "${GREEN}Skyring log directory: /var/log/skyring${NC}\n"
set -x
# Create logging directory
mkdir -p /var/log/skyring
chmod 777 /var/log/skyring

cd $HOME/golang/gopath/src/github.com/skyrings/skyring/python
python setup.py install
cd $HOME/golang/gopath/src/github.com/skyrings/skyring

rm -fr /tmp/.skyring-event

set +x
printf "${GREEN}Now the host setup is ready!${NC}\n"
printf "${GREEN}You can start the server by executing the following cmd:${NC}\n"
printf "${GREEN}PYTHONPATH=~/.local/lib/python2.7/site-packages skyring${NC}\n"
set -x

rm -f /usr/sbin/skyring
cp $HOME/golang/gopath/bin/skyring /usr/sbin/.
