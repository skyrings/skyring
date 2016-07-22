#!/bin/bash
USER=`/bin/mongo -quiet skyring --eval 'db.getUser("admin")["user"]'`
if [ $? -ne 0 ] || [ "${USER}" != 'admin' ]; then
    /bin/systemctl status mongod > /dev/null 2>&1
    if [ $? -eq 0 ]; then
	/bin/mongo skyring --eval 'db.createUser( { "user" : "admin", "pwd": "admin", "roles" : ["readWrite", "dbAdmin", "userAdmin"] })'
    fi
fi
sed -i -e 's/^#.*auth = true/auth = true/g' /etc/mongodb.conf
