#!/bin/sh
# This script is intended to be fully posix compliant.
# Script for updating and changing branch.

error() {
	printf "%s" "$1"
	exit 1
}

usage="create systemd service
example: $(basename "$0") --branch master
        --branch
            git branch.
        --repo
            git repository.
        -h, --help
            Show this help text.
"

# Root check.
if [ "$(id -u)" != 0 ]; then
	printf "Please run as root.\\n"
	exit 1
fi

# Parse arguments
branch=""
repo=""
for arg in "$@"; do
	case $arg in
	--branch)
		branch="$2"
		shift
		shift
		;;
	--branch=*)
		branch="${arg#*=}"
		shift
		;;
	--repo)
		repo="$2"
		shift
		shift
		;;
	--repo=*)
		repo="${arg#*=}"
		shift
		;;
	-h | --help)
		printf "%s" "$usage"
		shift
		exit 0
		;;
	esac
done

# Working directory check.
if [ ! -e "$(pwd)/go.mod" ]; then
	printf "The current working directory must be the project home and contain 'go.mod'\\n%s\\n" "$(pwd)"
	exit 1
fi

# Default values.
if [ "$branch" = "" ]; then
	branch="master"
fi
if [ "$repo" = "" ]; then
	repo="https://github.com/osnvr/os-nvr.git"
fi

# Check for local git changes.
if [ "$(git status --porcelain)" ]; then
	git status
	printf "Local changes found. Aborting..\\n"
	exit 1
fi

git remote add update "$repo" 2>/dev/null

sudo -u _nvr git fetch "$repo" "$branch" &&
	sudo -u _nvr git remote set-url update "$repo" &&
	sudo -u _nvr git fetch update "$branch" &&
	sudo -u _nvr git reset --hard update/"$branch" ||
	printf "Update failed.\\n" &&
	exit 1

printf "Update successful. Changes will be applied after restart.\\n"
