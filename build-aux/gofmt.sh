#!/bin/bash

files=$(git ls-files | grep '\.go$' | xargs gofmt -l -s -e | xargs)
if [ "$files" ]; then
    echo "ERROR: below source files not in golang format"
    echo $files | sed 's/ /\n/g'
    echo
    echo "To see suggested changes, run 'gofmt -d -e -s $files'"
    echo "To write suggested changes, run 'gofmt -e -s -w $files'"
    exit 1
fi

files=$(git ls-files --others | grep '\.go$' | xargs gofmt -l -s -e | xargs)
if [ "$files" ]; then
    echo "IGNORE: below untracked source files not in golang format"
    echo $files | sed 's/ /\n/g'
    echo
fi

exit 0
