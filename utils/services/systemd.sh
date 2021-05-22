#!/bin/sh
# This script is intended to be fully posix compliant.

error() {
	printf "%s" "$1"
	exit 1
}

usage="create systemd service
example: sudo $(basename "$0") \\
	--name=nvr \\
	--goBin \$(which go) \\
	--homeDir /home/_nvr/os-nvr \\
	--configDir /home/_nvr/os-nvr/configs

        --name
            service name.
        --goBin
            path to golang binary.
        --homeDir
            project home.
        --configDir
            configuration directory.
    -h, --help
            Show this help text
	 
"
#--cmd=/usr/bin/go run /home/_nvr/nvr/start/start.go

# Go to script location.
cd "$(dirname "$(readlink -f "$0")")" || error "could not got to script location"

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
home_dir=""
config_dir=""
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
	--configDir)
		config_dir="$2"
		shift
		shift
		;;
	--configDir=*)
		config_dir="${arg#*=}"
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
if [ "$home_dir" = "" ]; then
	printf "Error: --homeDir not specified\\n"
	exit 1
fi
if [ "$config_dir" = "" ]; then
	printf "Error: --configDir not specified\\n"
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

start_cmd="$go_bin run $home_dir/start/start.go --goBin $go_bin --homeDir $home_dir --configDir $config_dir"

# Fill in command.
sed -i "s:\$cmd:$start_cmd:" "$service_file"

# Enable and start service
systemctl enable "$name"
systemctl start "$name"
