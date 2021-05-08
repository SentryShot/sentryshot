#!/bin/sh
# This script is intended to be fully posix compliant.
# Print number of lines in this repository.

# Go to home.
script_path=$(readlink -f "$0")
home_dir=$(dirname "$(dirname "$script_path")")
cd "$home_dir" || exit

git ls-files | grep -E ".go$|.js$|.mjs$|.tpl$|.sh$" | grep -vE "vendor" | xargs wc -l
