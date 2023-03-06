#!/bin/sh
# This script is intended to be fully posix compliant.

# Go to home.
script_path=$(readlink -f "$0")
home_dir=$(dirname "$(dirname "$script_path")")
cd "$home_dir" || exit

mkdir -p ./coverage/report 2>/dev/null

exit_code=0

# Run tests.
go test -v -race -timeout=1s ./... || exit_code=1
#go test -v -race -timeout=1s ./... 2>&1 | go-junit-report >./coverage/report/go.xml || exit_code=1

# Generate test coverage report.
#go test -coverprofile=./coverage/gocover.txt -covermode count ./... || exit_code=1
#gocover-cobertura <./coverage/gocover.txt >./coverage/gocover.xml || exit_code=1

exit "$exit_code"
