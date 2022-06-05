#!/bin/sh
# This script is intended to be fully posix compliant.

error() {
	printf "%s" "$1"
	exit 1
}

usage="create systemd service
example: $(basename "$0") \\
    --name nvr \\
    --goBin \$(which go) \\
    --homeDir '/home/_nvr/os-nvr'

        --name
            service name.
        --goBin
            path to Golang binary.
        --homeDir
            project home.
        -h, --help
            Show this help text.
"

# Go to script location.
cd "$(dirname "$(readlink -f "$0")")" || error "could not go to script location"

# Root check.
if [ "$(id -u)" != 0 ]; then
	printf "Please run as root.\\n"
	exit 1
fi

# No arguments.
if [ -z "$*" ]; then
	printf "%s" "$usage"
	exit 1
fi

# Parse arguments
name=""
go_bin=""
home_dir=""
for arg in "$@"; do
	case $arg in
	--name)
		name="$2"
		shift
		shift
		;;
	--name=*)
		name="${arg#*=}"
		shift
		;;
	--goBin)
		go_bin="$2"
		shift
		shift
		;;
	--goBin=*)
		go_bin="${arg#*=}"
		shift
		;;
	--homeDir)
		home_dir="$2"
		shift
		shift
		;;
	--homeDir=*)
		home_dir="${arg#*=}"
		shift
		;;
	-h | --help)
		printf "%s" "$usage"
		shift
		exit 0
		;;
	esac
done

if [ "$name" = "" ]; then
	printf "Please specify --name\\n"
	exit 1
fi
if [ "$go_bin" = "" ]; then
	printf "Please specify --goBin\\n"
	exit 1
fi
if [ "$home_dir" = "" ]; then
	printf "Please specify --homeDir\\n"
	exit 1
fi

# Sanity check.
service_dir="/etc/systemd/system/"
service_file="$service_dir/$name.service"
if [ ! -d "$service_dir" ]; then
	printf "Error: could not find services directory: %s\\n" "$service_dir"
	exit 1
fi

if [ -f "$service_file" ]; then
	while true; do
		printf "service already exists, overwrite? [Y/N]"
		read -r overwrite
		case $overwrite in
		[Yy]*)
			rm "$service_file"
			break
			;;
		[Nn]*)
			exit 0
			;;
		esac
	done
fi

# Copy template to service_dir
cp ./templates/systemd.service "$service_file"

start_cmd="$home_dir/start/start.sh --goBin $go_bin"

# Fill in command and working directory.
sed -i "s:\$cmd:$start_cmd:" "$service_file"
sed -i "s:\$wd:$home_dir:" "$service_file"

# Enable and start service
systemctl enable "$name"
systemctl start "$name"
