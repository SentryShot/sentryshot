#!/bin/sh
# This script is intended to be fully posix compliant.
# List source files.

# Go to home.
script_path=$(readlink -f "$0")
home_dir=$(dirname "$(dirname "$script_path")")
cd "$home_dir" || exit

find . \
	-type f -name "$1" \
	-not -path "./.git/*" \
	-not -path "./.cache/*" \
	-not -path "*vendor*" \
	-not -path "./node_modules/*" \
	-not -path "./local/*" \
	-not -path "./start/build/main.go"
