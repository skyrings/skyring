#!/bin/bash

# To override version/release from git,
# create VERSION file containing text with version/release
# eg. v3.4.0-1

if [ -f VERSION ]; then
    PKG_VERSION=$(cat VERSION)
else
    PKG_VERSION=$(git describe --tags --match "v[0-9]*" 2>/dev/null)
fi

if [ -z "$PKG_VERSION" ]; then
    hash=$(git log -n 1 --format=%h 2>/dev/null)
    patch_count=$(git rev-list HEAD --count)
    PKG_VERSION=$(echo -n v0.0.0-$patch_count-$hash)
fi

function get_version ()
{
    # tags and output versions:
    #   - v3.4.0   => 3.4.0 (upstream clean)
    #   - v3.4.0-1 => 3.4.0 (downstream clean)
    #   - v3.4.0-2-g34e62f   => 3.4.0 (upstream dirty)
    #   - v3.4.0-1-2-g34e62f => 3.4.0 (downstream dirty)
    AWK_VERSION='
    BEGIN { FS="-" }
    /^v[0-9]/ {
      sub(/^v/,"") ; print $1
    }'

    echo $PKG_VERSION | awk "$AWK_VERSION"
}

function get_release ()
{
    # tags and output releases:
    #   - v3.4.0   => 0 (upstream clean)
    #   - v3.4.0-1 => 1 (downstream clean)
    #   - v3.4.0-2-g34e62f1   => 2.git34e62f1 (upstream dirty)
    #   - v3.4.0-1-2-g34e62f1 => 1.2.git34e62f1 (downstream dirty)
    AWK_RELEASE='
    BEGIN { FS="-"; OFS="." }
    /^v[0-9]/ {
      if (NF == 1) print 0
      else if (NF == 2) print $2
      else if (NF == 3) print $2, "git" substr($3, 2)
      else if (NF == 4) print $2, $3, "git" substr($4, 2)
    }'

    echo $PKG_VERSION | awk "$AWK_RELEASE"
}

VERSION=$(get_version)
RELEASE=$(get_release)

cat >$1 <<EOF
package main

// VERSION package version number
const VERSION = "$VERSION"

// RELEASE package release number
const RELEASE = "$RELEASE"
EOF

exit 0
