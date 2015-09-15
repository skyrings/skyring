
function check_golang_env()
{
    if [ -z "$GOROOT" ]; then
        echo "GOROOT environment variable is not set.  exiting"
        exit 1
    fi

    if [ -z "$GOPATH" ]; then
        echo "GOPATH environment variable is not set.  exiting"
        exit 1
    fi

    if ! which go >/dev/null 2>&1; then
        echo "go executable is not found.  exiting"
        exit 1
    fi
}

main()
{
    check_golang_env
}

main "$@"
