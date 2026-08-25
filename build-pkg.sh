#!/bin/bash

# Build an RPM package containing dbtail, its configuration, systemd unit,
# schemas, and RPM-specific installation notes.
set -euo pipefail

usage() {
    echo "Usage: $0 -v <version> [-r <release>] [-t rpm]"
    exit 2
}

version=""
release="1"
pkg_type="rpm"
while getopts "v:r:t:" opt; do
    case "$opt" in
        v) version=$OPTARG ;;
        r) release=$OPTARG ;;
        t) pkg_type=$OPTARG ;;
        *) usage ;;
    esac
done

if [ -z "$version" ] || [ "$pkg_type" != "rpm" ]; then
    usage
fi

repo_dir=$(cd "$(dirname "$0")" && pwd)
binary="$repo_dir/build/dbtail-linux-amd64"
output_dir="$repo_dir/build/packages"

if [ ! -x "$binary" ]; then
    echo "Missing Linux amd64 binary: $binary" >&2
    echo "Build it before packaging." >&2
    exit 1
fi

nfpm_cmd=""
if command -v nfpm >/dev/null 2>&1; then
    nfpm_cmd=$(command -v nfpm)
elif [ -x "$repo_dir/build/tools/nfpm" ]; then
    nfpm_cmd="$repo_dir/build/tools/nfpm"
else
    echo "nFPM is required to build the RPM package." >&2
    exit 1
fi

mkdir -p "$output_dir"

cd "$repo_dir"
PACKAGE_VERSION="$version" PACKAGE_RELEASE="$release" "$nfpm_cmd" package \
    --config packaging/rpm/nfpm.yaml \
    --packager rpm \
    --target "$output_dir/dbtail-${version}-${release}.x86_64.rpm"

echo "Created $output_dir/dbtail-${version}-${release}.x86_64.rpm"
