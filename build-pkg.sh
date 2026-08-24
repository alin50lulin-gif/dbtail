#!/bin/bash

# Build deb or rpm packages for dbtail.
set -e

function usage() {
    echo "Usage: build-pkg.sh -v <version> -t <package_type>"
    exit 2
}

while getopts "v:t:" opt; do
    case "$opt" in
    v)
        version=$OPTARG
        ;;
    t)
        pkg_type=$OPTARG
        ;;
    esac
done

if [ -z "$version" ] || [ -z "$pkg_type" ]; then
    usage
fi

fpm -s dir -n dbtail \
    -m "Support <support@altinity.com>" \
    -p $GOPATH/bin \
    -v $version \
    -t $pkg_type \
    --pre-install=./preinstall \
    $GOPATH/bin/dbtail=/usr/bin/dbtail \
    ./dbtail.upstart=/etc/init/dbtail.conf \
    ./dbtail.service=/lib/systemd/system/dbtail.service \
    ./dbtail.conf=/etc/dbtail/dbtail-example.conf
