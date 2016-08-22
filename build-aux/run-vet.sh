#!/bin/bash

go tool vet ./main.go
for ENTRY in `ls . | grep -v '^vendor'`
do
	if [ -d ${ENTRY} ]
	then
		go tool vet $ENTRY
	fi
done
