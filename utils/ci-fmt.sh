#!/bin/sh
# This script is intended to be fully posix compliant.
# Run all formatters, linters and tests.

error() {
	printf "%s\\n" "$1"
	exit 1
}

# Go to home.
script_path=$(readlink -f "$0")
home_dir=$(dirname "$(dirname "$script_path")")
cd "$home_dir" || error "could not go to home"

files="$1"
if [ "$files" = "" ]; then
	files=".go .mjs .css .sh"
fi
modified() {
	pattern=${1}
	test "${files#*$pattern}" != "$files"
}

modified ".go" && {
	printf "format go\\n"
	./utils/format-go.sh || error "format go failed"
	printf "lint go\\n"
	./utils/lint-go.sh || error "lint go failed"
	printf "test go\\n"
	./utils/test-go.sh || error "test go failed"
}

modified ".sh" && {
	printf "format shell\\n"
	./utils/format-shell.sh || error "format shell failed"
	printf "lint shell\\n"
	./utils/lint-shell.sh || error "lint shell failed"
}

modified ".js$" || modified ".mjs" || modified ".css" && {
	printf "format frontend\\n"
	./utils/format-frontend.sh || error "format frontend failed"
}

modified ".js$" || modified ".mjs" && {
	./utils/lint-js-fix.sh || error "lint js failed"
	printf "test js\\n"
	./utils/test-js.sh || error "test js failed"
}

modified ".css" && {
	./utils/lint-css-fix.sh || error "lint css failed"
}

printf "spell check\\n"
./utils/spell-check-fix.sh "$1" || error "spell check failed"

printf "all passed!\\n"
