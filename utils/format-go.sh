#!/bin/sh
# This script is intended to be fully posix compliant.

# Go to home.
script_path=$(readlink -f "$0")
home_dir=$(dirname "$(dirname "$script_path")")
cd "$home_dir" || exit

exit_code=0

# Find files.
files=$(find . -type f -name "*.go" -not -path "./vendor/_")
for file in $files; do
	goimports -w "$file" || exit_code=1
done

exit "$exit_code"
