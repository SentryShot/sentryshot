#!/bin/sh

set -e

ci_version="v0.12.3"

script_dir=$(dirname "$(readlink -f "$0")")
cd "$script_dir"

docker image build -t osnvr/os-nvr_ci:latest .
docker push osnvr/os-nvr_ci:latest

docker image build -t osnvr/os-nvr_ci:$ci_version .
docker push osnvr/os-nvr_ci:$ci_version
