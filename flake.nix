{
  description = "A flake for sentryshot project";

  inputs = {
    # glibc 2.34 nixos-22.05
    # https://github.com/NixOS/nixpkgs/commits/380be19fbd2d9079f677978361792cb25e8a3635
    nixpkgs.url = "github:nixos/nixpkgs/380be19fbd2d9079f677978361792cb25e8a3635";
  };

  outputs = { self, nixpkgs }:
  let
    # --- Project-wide constants ---------------------------------
    projectRoot = ./.;
    supportedSystems = [ "x86_64-linux" ];

    cargoTomlContent = builtins.readFile ./Cargo.toml;
    projectMetadata = builtins.fromTOML cargoTomlContent;
    rustVersion = "1.85.0";

    # Helper: import pkgs for a given system with the project's overlay
    pkgsFor = system: import nixpkgs {
      inherit system;
      overlays = [ (import (projectRoot + "/misc/nix/rust-overlay")) ];
    };

    # Helper: consistent Rust platform
    mkRustPlatform = pkgs: pkgs.makeRustPlatform {
      cargo = pkgs.rust-bin.stable.${rustVersion}.minimal;
      rustc = pkgs.rust-bin.stable.${rustVersion}.minimal;
    };

    # Build the sentryshot package for a given system
    makeSentry = system: let
      pkgs = pkgsFor system;
      ffmpeg = pkgs.callPackage ./misc/nix/ffmpeg.nix {};
      tflite = pkgs.callPackage ./misc/nix/tflite.nix {};
      libedgetpu = pkgs.callPackage ./misc/nix/libedgetpu.nix {};
      rustPlatform = mkRustPlatform pkgs;
    in rustPlatform.buildRustPackage {
      pname = "sentryshot";
      version = projectMetadata.workspace.package.version;
      src = projectRoot;

      buildInputs = [ ffmpeg tflite libedgetpu pkgs.libusb1 ];

      nativeBuildInputs = [ pkgs.pkg-config pkgs.protobuf ];

      cargoLock = {
        lockFile = ./Cargo.lock;
        outputHashes = {
          "retina-0.4.11" = "sha256-BLvE4wo5DeijfADcGQczYrmLgzb0vOr6Pl+Y+ERbj5U=";
        };
      };

      meta = with pkgs.lib; {
        description = "Video Management System";
        homepage = "https://codeberg.org/SentryShot/sentryshot";
        license = licenses.gpl2Plus;
      };
    };

    # Build a docker image for a given system
    makeDockerImage = system: let
      pkgs = pkgsFor system;
      sentry = self.packages.${system}.sentryshot;
      ffmpeg = pkgs.callPackage ./misc/nix/ffmpeg.nix {};
      tflite = pkgs.callPackage ./misc/nix/tflite.nix {};
      libedgetpu = pkgs.callPackage ./misc/nix/libedgetpu.nix {};
    in pkgs.dockerTools.buildImage {
      name = "henryouly/sentryshot";
      tag = "latest";

      contents = pkgs.buildEnv {
        name = "image-root";
        paths = [
          sentry
          ffmpeg
          tflite
          libedgetpu
          pkgs.libusb1
          pkgs.openh264
          pkgs.bash
          pkgs.glibc
        ];
      };

      config = {
        Entrypoint = [ "${sentry}/bin/sentryshot" ];
        Cmd = [ "" ];
        Env = [
          "LD_LIBRARY_PATH=${pkgs.lib.makeLibraryPath [ ffmpeg tflite libedgetpu pkgs.libusb1 pkgs.openh264 ]}"
          "TZ=America/Los_Angeles"
        ];
        ExposedPorts = { "2020/tcp" = {}; };
        Volumes = { "/configs" = {}; "/storage" = {}; };
      };
    };

    # Dev shell for a given system
    makeDevShell = system: let
      pkgs = pkgsFor system;
      ffmpeg = pkgs.callPackage ./misc/nix/ffmpeg.nix {};
      tflite = pkgs.callPackage ./misc/nix/tflite.nix {};
      libedgetpu = pkgs.callPackage ./misc/nix/libedgetpu.nix {};
    in pkgs.mkShell {
      nativeBuildInputs = [ pkgs.rust-bin.stable.${rustVersion}.minimal pkgs.pkg-config pkgs.protobuf ];
      buildInputs = [ ffmpeg tflite libedgetpu pkgs.libusb1 pkgs.openh264 ];

      LD_LIBRARY_PATH = "${ffmpeg}/lib:${tflite}/lib:${libedgetpu}/lib:${pkgs.libusb1}/lib:${pkgs.openh264}/lib";

      FFLIBS = "${ffmpeg}/lib";
      TFLITELIB = "${tflite}/lib";
      EDGETPULIB= "${libedgetpu}/lib";
      OPENH264LIB= "${pkgs.openh264}/lib";
      CARGO_BUILD_TARGET = "x86_64-unknown-linux-gnu";
      CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_LINKER = "${pkgs.stdenv.cc.targetPrefix}cc";
      CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_RUSTFLAGS = "-C link-args=-Wl,-rpath,$ORIGIN/libs:$ORIGIN/../libs";
    };

  in
  {
    packages = {
      x86_64-linux = {
        sentryshot = makeSentry "x86_64-linux";
        dockerImage = makeDockerImage "x86_64-linux";
        default = makeSentry "x86_64-linux";
      };
    };

    devShells = {
      "x86_64-linux" = { default = makeDevShell "x86_64-linux"; };
    };
  };
}
