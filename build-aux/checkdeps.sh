#!/bin/bash

function check_golang_env()
{
    #if [ -z "$GOROOT" ]; then
    #    echo "ERROR: GOROOT environment variable is not set"
    #    exit 1
    #fi

    if [ -z "$GOPATH" ]; then
        echo "ERROR: GOPATH environment variable is not set"
        exit 1
    fi

    if ! which go >/dev/null 2>&1; then
        echo "ERROR: go executable not found in PATH environment"
        exit 1
    fi

    if [ "$PWD" != "$GOPATH/src/github.com/skyrings/skyring" ]; then
        echo "ERROR: project not found in $GOPATH/src/github.com/skyrings/skyring"
        exit 1
    fi
}

main()
{
    check_golang_env
}

main "$@"
