#!/bin/sh

set -e

script_dir=$(dirname "$(readlink -f "$0")")
cd "$script_dir"

docker image build -t osnvr/os-nvr_ci .
