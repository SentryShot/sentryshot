#!/bin/sh
# This script is intended to be fully posix compliant.

set -e

usage="Utilities
  Commands:
    help           # Display this help and exit.
    build-target-nix <TARGET> # Build target using nix shell.
    mount-tmpfs  # Mount tmpfs to the build directory.
    umount-tmpfs # Unmount the tmpfs.
    dev-env-nix    # Enter a shell with all deps installed.
    dev-env-docker # Enter a container with all deps installed.
    test-stream    # Test stream container rtsp://127.0.0.1:8554/1
"

# Go to project root.
script_path=$(readlink -f "$0")
root_dir=$(dirname "$(dirname "$script_path")")
cd "$root_dir"


target_dir="$(pwd)/target2"
if [ "$CARGO_TARGET_DIR" ]; then
	target_dir=$CARGO_TARGET_DIR
fi

if [ -z "${CARGO_TARGET_DIR}" ]; then
	export CARGO_TARGET_DIR="$target_dir"
fi

parse_command() {
	case $1 in
	help)
		printf "%s" "$usage"
		exit 0
		;;
	build-target-nix)
		shift
		if [ "$#" -ne 1 ]; then
			printf "missing target: 'x86_64', 'aarch64'\n"
			exit 1
		fi
		target=$*
		nix-shell --pure ./misc/nix/shell-"$target".nix --run "./misc/utils.sh build-target $target"
		exit 0
		;;
	mount-tmpfs)
		shift
		mkdir -p ./target
		mkdir -p ./build
		mount -t tmpfs -o size=8G none ./target2
		mount -t tmpfs -o size=500M none ./build
		df -h
		exit 0
		;;
	umount-tmpfs)
		shift
		umount ./target2
		umount ./build
		df -h
		exit 0
		;;
	dev-env-nix)
		shift
		CARGO_TARGET_DIR="$target_dir" nix-shell ./misc/nix/shell-ci.nix
		exit 0
		;;
	dev-env-docker)
		shift
		docker run -it \
			-u "$(id -u)" \
			-p 2020:2020 \
			-v "$(pwd)":/app \
			-v sentryshot-dev:/root \
			--workdir /app \
			--entrypoint nix-shell \
			codeberg.org/sentryshot/sentryshot-ci:v0.3.0 \
			/shell.nix
		exit 0
		;;
	download-debian-libusb)
		shift
		download_debian_libusb
		exit 0
		;;
	test-stream)
		shift
		docker run -it --network=host codeberg.org/sentryshot/test-stream:v0.1.0
		exit 0
		;;
	esac
	
	printf "%s" "$usage"

	# Remove in v0.4.0
	./misc/make.sh "$@"
}

download_debian_libusb() {
	if [ -d "./libusb" ]; then
		printf "'./libusb' directory already exists"
		exit 1
	fi

	# amd64
	mkdir -p "./libusb/temp"
	wget "http://ftp.de.debian.org/debian/pool/main/libu/libusb-1.0/libusb-1.0-0_1.0.26-1_amd64.deb" -O "./libusb/temp/libusb.deb"
	if ! printf "0a8a6c4a7d944538f2820cbde2a313f2fe6f94c21ffece9e6f372fc2ab8072e1 ./libusb/temp/libusb.deb" | sha256sum -c; then
		printf "invalid amd64 libusb checksum\n"
		exit 1
	fi
	dpkg-deb -X "./libusb/temp/libusb.deb" "./libusb/temp"
	cp -r "./libusb/temp/usr/lib/x86_64-linux-gnu" "./libusb/"
	rm -r "./libusb/temp"
	
	# aarch64
	mkdir -p "./libusb/temp"
	wget "http://ftp.de.debian.org/debian/pool/main/libu/libusb-1.0/libusb-1.0-0_1.0.26-1_arm64.deb" -O "./libusb/temp/libusb.deb"
	if ! printf "e0648086b231c9204514d31480d517cb1b51e301ac39e69335a67d01ec785608 ./libusb/temp/libusb.deb" | sha256sum -c; then
		printf "invalid aarch64 libusb checksum\n"
		exit 1
	fi
	dpkg-deb -X "./libusb/temp/libusb.deb" "./libusb/temp"
	cp -r "./libusb/temp/usr/lib/aarch64-linux-gnu" "./libusb/"
	rm -r "./libusb/temp"
}

parse_command "$@"
