#!/bin/sh
# This script is intended to be fully posix compliant.

set -e

usage="Utilities
  Commands:
    $0 help         # Display this help and exit.
    $0 run <args>   # Build and run app.
    $0 run run      # Example run command.
	$0 run-release  # Run in release mode.
    $0 build-target <TARGET> # Build target.
    $0 build-target-nix # Same as above but using nix shell. 
    $0 test-backend  # Run backend tests.
    $0 lint-backend  # Run backend linters.
    $0 lint-frontend # Run frontend linters.
    $0 ci           # Full CI suite without file changes.
    $0 ci-fix       # Full CI suite.
    $0 ci-frontend  # Frontend CI suite.
    $0 ci-backend   # Backend CI suite.
    $0 mount-tmpfs  # Mount tmpfs to the build directory.
    $0 umount-tmpfs # Unmount the tmpfs.
    $0 dev-env-nix  # Enter a shell with all deps installed.
    $0 dev-env-docker # Enter a container with all deps installed.
    $0 clean          # Clean build directories.
  Use 'npm run cover x' to run javascript tests
"

# Go to project root.
script_path=$(readlink -f "$0")
root_dir=$(dirname "$(dirname "$script_path")")
cd "$root_dir"

# Create version file if it doesn't exist.
version_file="./misc/version"
if [ ! -e "$version_file" ]; then
	touch "$version_file"
fi

target_dir="$(pwd)/target2"
if [ "$CARGO_TARGET_DIR" ]; then
	target_dir=$CARGO_TARGET_DIR
fi

if [ -z "${CARGO_TARGET_DIR}" ]; then
	export CARGO_TARGET_DIR="$target_dir"
fi

plugins="auth_basic auth_none motion mqtt object_detection thumb_scale openvino"
packages="-p sentryshot"
for plugin in $plugins; do
	packages="$packages -p $plugin"
done

parse_command() {
	case $1 in
	run)
		shift
		# shellcheck disable=SC2086,SC2091
		$(cargo build $packages)
		update_plugin_dir "debug"
		"$target_dir"/debug/sentryshot "$@"
		exit 0
		;;
	run-release)
		shift
		# shellcheck disable=SC2086,SC2091
		$(cargo build --release $packages)
		update_plugin_dir "release"
		"$target_dir"/release/sentryshot "$@"
		exit 0
		;;
	build-target)
		shift
		if [ "$#" -ne 1 ]; then
			printf "missing target: 'x86_64', 'aarch64'\n"
			exit 1
		fi
		build_target "$@"
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
	test-backend | test-back | test-be | test-b | testb)
		shift
		test_backend
		exit 0
		;;
	lint-backend | lint-back | lint-be | lint-b | lintb)
		shift
		lint_backend
		exit 0
		;;
	lint-frontend | lint-front | lint-fe | lint-f | lintf)
		shift
		lint_css_fix
		lint_js_fix
		exit 0
		;;
	ci)
		shift
		lint_js
		test_js
		lint_css
		format_backend
		lint_backend
		test_backend
		printf "all passed!"
		exit 0
		;;
	ci-fix)
		shift
		format_frontend
		lint_js_fix
		test_js
		lint_css_fix
		format_backend_fix
		lint_backend_fix
		test_backend
		printf "all passed!"
		exit 0
		;;
	ci-frontend | ci-front | ci-fe | ci-f | cif)
		shift
		format_frontend
		lint_js_fix
		test_js
		lint_css_fix
		exit 0
		;;
	ci-backend | ci-back | ci-be | ci-b | cib)
		shift
		format_backend
		lint_backend
		test_backend
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
	clean)
		shift
		rm -r ./target/* || true
		rm -r ./target2/* || true
		rm -r ./build/* || true
		cargo clean
		exit 0
		;;
	download-debian-libusb)
		shift
		download_debian_libusb
		exit 0
		;;
	esac
	printf "%s" "$usage"
}

update_plugin_dir() {
	mode="$1"
	rm ./plugin_dir || true
	ln -sf "$target_dir/$mode" ./plugin_dir
}

build_target() {
	target=$1
	shift
	
	printf "build\n"
	# shellcheck disable=SC2086,SC2091
	$(cargo build --release --target="$target"-unknown-linux-gnu $packages)

	output_dir=./build/"$target"
	mkdir -p "$output_dir/plugins"

	printf "copy files\n"
	printf "./build/%s/sentryshot\n" "$target"
	output="$output_dir"/sentryshot
	cp "$target_dir"/"$target"-unknown-linux-gnu/release/sentryshot "$output"
	patch_rpath "$output"
	patch_interpreter "$output" "$target"

	# Copy plugins.
	for plugin in $plugins; do
		printf "./build/%s/plugins/libsentryshot_%s.so\n" "$target" "$plugin"
		# Cargo doesn't let you specify the output file: https://github.com/rust-lang/cargo/issues/9778
		output="$output_dir/plugins/libsentryshot_$plugin.so"
		cp "$target_dir"/"$target"-unknown-linux-gnu/release/lib"$plugin".so "$output"
		patch_rpath "$output"
	done

	# Copy libs.
	mkdir -p "$output_dir"/libs
	printf "./build/%s/libs/libavutil.so\n" "$target"
	cp "$FFLIBS"/libavutil.so.?? "$output_dir"/libs/
	printf "./build/%s/libs/libavcodec.so\n" "$target"
	cp "$FFLIBS"/libavcodec.so.?? "$output_dir"/libs/
	printf "./build/%s/libs/libtensorflowlite_c.so\n" "$target"
	cp "$TFLITELIB"/libtensorflowlite_c.so "$output_dir"/libs/
	printf "./build/%s/libs/libedgetpu.so.1\n" "$target"
	cp "$EDGETPULIB"/libedgetpu.so.1.0 "$output_dir"/libs/libedgetpu.so.1
	printf "./build/%s/libs/libopenh264.so.6\n" "$target"
	cp "$OPENH264LIB"/libopenh264.so.6 "$output_dir"/libs/
	chmod 644 "$output_dir"/libs/*

	for lib in "$output_dir"/libs/*; do
		# Remove the nix runpath.
		patchelf --remove-rpath "$lib"
	done
}

# Removes the nix interpreter prefix.
patch_interpreter() {
	file=$1
	target=$2

	case $target in
	x86_64)
		patchelf --set-interpreter "/lib64/ld-linux-x86-64.so.2" "$file"
		;;
	aarch64)
		patchelf --set-interpreter "/lib/ld-linux-aarch64.so.1" "$file"
		;;
	esac
}

patch_rpath() {
	# shellcheck disable=SC2016
	patchelf --set-rpath '$ORIGIN/libs:$ORIGIN/../libs' "$1"
}

format_frontend() {
	printf "format frontend\\n"
	npm run format || error "format frontend failed"
}

lint_js() {
	printf "lint js\\n"
	npm run ts
	npm run lint-js || error "lint js failed"
}

lint_js_fix() {
	printf "lint js\\n"
	npm run ts
	npm run lint-js-fix || error "lint js failed"
}

test_js() {
	printf "test js\\n"
	npm run test || error "test js failed"
}

lint_css() {
	printf "lint css\\n"
	npm run lint-css || error "lint css failed"
}

lint_css_fix() {
	printf "lint css\\n"
	npm run lint-css-fix || error "lint css failed"
}

format_backend() {
	printf "format backend\\n"
	cargo fmt --check --all || error "format backend failed"
}

format_backend_fix() {
	printf "format backend\\n"
	cargo fmt --all || error "format backend failed"
}

lint_backend() {
	printf "lint backend\\n"
	cargo clippy --workspace --no-deps --all-targets -- -Dwarnings || error "clippy failed"
	git ls-files | grep \.sh$ | xargs shellcheck || error "shellcheck failed"
}

lint_backend_fix() {
	printf "lint backend\\n"
	cargo clippy --workspace --no-deps --all-targets --fix --allow-staged --allow-dirty || error "clippy failed"
	cargo clippy --workspace --no-deps --all-targets -- -Dwarnings || error "clippy failed"
	git ls-files | grep \.sh$ | xargs shellcheck || error "shellcheck failed"
}

test_backend() {
	printf "test backend\\n"
	cargo test --workspace || error "test backend"
}

error() {
	printf "%s\\n" "$1"
	exit 1
}

download_debian_libusb() {
	printf "aa\\n"
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
