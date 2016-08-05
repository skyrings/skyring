#!/bin/bash
USER=`/bin/mongo -quiet skyring --eval 'db.getUser("admin")["user"]'`
if [ $? -ne 0 ] || [ "${USER}" != 'admin' ]; then
    /bin/systemctl status mongod > /dev/null 2>&1
    if [ $? -eq 0 ]; then
	pwd=`python -c 'import json;print json.loads(open("/etc/skyring/skyring.conf").read())["dbconfig"]["password"]'`
	cmd="/bin/mongo skyring --eval 'db.createUser( { \"user\" : \"admin\", \"pwd\": \"$pwd\", \"roles\" : [\"readWrite\", \"dbAdmin\", \"userAdmin\"] })'"
	eval $cmd
    fi
fi
exit 0
