#!/bin/sh
# This script is intended to be fully posix compliant.

# Go to home.
script_path=$(readlink -f "$0")
home_dir=$(dirname "$(dirname "$script_path")")
cd "$home_dir" || exit

exit_code=0

packages=$(go list ./...)
for p in $packages; do
	go test -race "$p" -timeout=1s -cover || exit_code=1
done

exit "$exit_code"
