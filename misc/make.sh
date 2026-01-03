#!/bin/sh

# Go to project root.
script_path=$(readlink -f "$0")
root_dir=$(dirname "$(dirname "$script_path")")
cd "$root_dir" || exit 1

cargo run -p make "$@"
