#!/bin/sh
# This script is intended to be fully posix compliant.

error() {
	printf "%s" "$1"
	exit 1
}

usage="create systemd service
example: sudo $(basename "$0") \\
    --name nvr \\
    --goBin \$(which go) \\
    --env '/home/_nvr/os-nvr/configs/env.yaml' \\
    --homeDir '/home/_nvr/os-nvr'

        --name
            service name.
        --goBin
            path to golang binary.
        --env
            path to env.yaml.
        --homeDir
            project home.
        -h, --help
            Show this help text.
"
#--cmd=/usr/bin/go run /home/_nvr/nvr/start/start.go

# Go to script location.
cd "$(dirname "$(readlink -f "$0")")" || error "could not go to script location"

# Root check.
if [ "$(id -u)" != 0 ]; then
	printf "Please run as root\\n"
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
env=""
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
	--env)
		env="$2"
		shift
		shift
		;;
	--env=*)
		env="${arg#*=}"
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
	printf "Error: --name not specified\\n"
	exit 1
fi
if [ "$go_bin" = "" ]; then
	printf "Error: --goBin not specified\\n"
	exit 1
fi
if [ "$env" = "" ]; then
	printf "Error: --env not specified\\n"
	exit 1
fi
if [ "$home_dir" = "" ]; then
	printf "Error: --homeDir not specified\\n"
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

start_cmd="$go_bin run $home_dir/start/start.go --env $env"

# Fill in command and working directory.
sed -i "s:\$cmd:$start_cmd:" "$service_file"
sed -i "s:\$wd:$home_dir:" "$service_file"

# Enable and start service
systemctl enable "$name"
systemctl start "$name"
