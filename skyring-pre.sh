#!/bin/bash
pwd=`python -c 'import json;print json.loads(open("/etc/skyring/skyring.conf").read())["dbconfig"]["password"]'`
# Check the db status
/bin/systemctl status mongod > /dev/null 2>&1
if [ $? -eq 0 ]; then
    # Check the db connection and fetch the user name
    UNAME=`/bin/mongo -quiet skyring --eval 'db.getUser("admin")["user"]' -u admin -p ${pwd}`
    if [ $? -ne 0 ]; then
	# Reset the auth flag to false because the password is changed
	/bin/grep "^auth = true" /etc/mongodb.conf > /dev/null 2>&1
	if [ $? -eq 0 ]; then
	    sed -i -e 's/auth = true/#auth = true/g' /etc/mongodb.conf
	    /bin/systemctl restart mongod > /dev/null 2>&1
	fi
	/bin/systemctl status mongod > /dev/null 2>&1
	if [ $? -eq 0 ]; then
	    # Check admin user exists already
	    USERNAME=`/bin/mongo -quiet skyring --eval 'db.getUser("admin")["user"]'`
	    if [ $? -ne 0 ] || [ "${USERNAME}" != 'admin' ]; then
		# Admin user does not exists
		cmd="/bin/mongo skyring --eval 'db.createUser( { \"user\" : \"admin\", \"pwd\": \"$pwd\", \"roles\" : [\"readWrite\", \"dbAdmin\", \"userAdmin\"] })'"
	    else
		# Admin user already exists. Update the password
		cmd="/bin/mongo skyring --eval 'db.changeUserPassword(\"admin\", \"${pwd}\")'"
	    fi
	    eval $cmd > /dev/null 2>&1 || :
	    # Update the security flag and restart the mongod service
	    /bin/grep "^#auth = true" /etc/mongodb.conf > /dev/null 2>&1
	    if [ $? -eq 0 ]; then
		sed -i -e 's/#auth = true/auth = true/g' /etc/mongodb.conf
	    fi
	    /bin/systemctl restart mongod > /dev/null 2>&1
	fi
    fi
fi
