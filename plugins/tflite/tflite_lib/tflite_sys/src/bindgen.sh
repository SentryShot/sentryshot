#!/bin/sh

set -e

# Go to script dir.
script_path=$(readlink -f "$0")
script_dir=$(dirname "$script_path")
cd "$script_dir"


bindgen ./wrapper.h \
	-o ./bindings.rs \
	--raw-line '#![allow(unused, non_snake_case, non_camel_case_types, non_upper_case_globals)]'

