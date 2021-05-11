#!/bin/sh
# This script is intended to be fully posix compliant.
# Run all formatters, linters and tests.

error() {
	printf "%s" "$1"
	exit 1
}

# Go to home.
script_path=$(readlink -f "$0")
home_dir=$(dirname "$(dirname "$script_path")")
cd "$home_dir" || error "could not go to home"

files="$1"
modified() {
	case "$files" in
	*$1*) return 1 ;;
	*) return 0 ;;
	esac
}

modified ".go" || (
	printf "format go\\n"
	./utils/format-go.sh || error "format go failed"
	printf "lint go\\n"
	./utils/lint-go.sh || error "lint go failed"
	printf "test go\\n"
	./utils/test-go.sh || error "test go failed"
)

modified ".sh" || (
	printf "format shell\\n"
	./utils/format-shell.sh || error "format shell failed"
	printf "lint shell\\n"
	./utils/lint-shell.sh || error "lint shell failed"
)

modified ".js$" || modified ".css" || (
	printf "format frontend\\n"
	./utils/format-frontend.sh || error "format frontend failed"
)

modified ".js$" || (
	./utils/lint-js.sh || error "lint js failed"
	printf "test js\\n"
	./utils/test-js.sh || error "test js failed"
)

modified ".css" || (
	./utils/lint-css.sh || error "lint css failed"
)

printf "all passed!\\n"
