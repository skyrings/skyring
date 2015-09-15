#!/bin/bash

files=$(git ls-files | grep '\.go$' | xargs gofmt -l -s -e)
if [ "$files" ]; then
    echo "ERROR: below source files not in golang format"
    echo $files | sed 's/ /\n/g'
    echo
    exit 1
fi

files=$(git ls-files --others | grep '\.go$' | xargs gofmt -l -s -e)
if [ "$files" ]; then
    echo "IGNORING: below untracked source files not in golang format"
    echo $files | sed 's/ /\n/g'
    echo
fi

exit 0
