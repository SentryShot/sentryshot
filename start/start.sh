#!/bin/sh
# This script is intended to be fully posix compliant.
# Main script used to start the program.

usage="create systemd service
example: $(basename "$0") --goBin /usr/bin/go --env /home/_nvr/os-nvr/configs/env.yaml
        --goBin
            path to Golang binary.
        --env
            path to env.yaml.
        -h, --help
            Show this help text.
"

# Parse arguments
go_bin=""
env=""
for arg in "$@"; do
	case $arg in
	--goBin)
		go_bin="$2"
		shift
		shift
		;;
	--goBin=*)
		go_bin="${arg#*=}"
		shift
		;;
	--env)
		env="$2"
		shift
		shift
		;;
	--env=*)
		env="${arg#*=}"
		shift
		;;
	-h | --help)
		printf "%s" "$usage"
		shift
		exit 0
		;;
	esac
done

# User check.
if [ "$(whoami)" != "_nvr" ]; then
	printf "Please run as user '_nvr'   \"sudo -u _nvr ./%s\"\\n" "$(basename "$0")"
	exit 1
fi

# Working directory check.
if [ ! -e "$(pwd)/go.mod" ]; then
	printf "The current working directory must be the project home and contain 'go.mod'\\n%s\\n" "$(pwd)"
	exit 1
fi

# Auto detect go_bin.
if [ "$go_bin" = "" ]; then
	go_bin="$(which go)"
	if [ "$go_bin" = "" ]; then
		printf "Could not determine Golang binary location. Please specify it using --goBin\\n"
		exit 1
	fi
	printf "goBin not speified. Using \"%s\"" "$go_bin"
fi

if [ "$env" = "" ]; then
	env="$(pwd)/configs/env.yaml"
	printf "env file not specified. Using \"%s\"\\n" "$env"
fi

# Start program.
$go_bin run ./start/start.go --env "$env"
