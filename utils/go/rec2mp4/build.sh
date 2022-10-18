#!/bin/sh

set -e

script_path=$(readlink -f "$0")
script_dir=$(dirname "$script_path")
cd "$script_dir"
mkdir -p dist

# Go to home.
home_dir=$(dirname "$(dirname "$script_path")")
cd "$home_dir" || exit

go build -o "$script_dir/dist/" "$script_dir/"
