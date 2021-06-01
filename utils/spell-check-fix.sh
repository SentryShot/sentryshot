#!/bin/sh
# This script is intended to be fully posix compliant.

# Go to home.
script_path=$(readlink -f "$0")
home_dir=$(dirname "$(dirname "$script_path")")
cd "$home_dir" || exit

exit_code=0

files="$1"
if [ "$files" = "" ]; then
	# Find files.
	files=$(find . -type f -not -path "./.git/*" -not -path "./.cache/*" -not -path "vendor" -not -path "./node_modules/*" -not -path "./local/*")
fi
for file in $files; do
	misspell -w "$file" || exit_code=1
done

return "$exit_code"
