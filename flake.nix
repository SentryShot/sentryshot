{
  description = "A flake for sentryshot project";

  inputs = {
    # glibc 2.34 nixos-22.05
    # https://github.com/NixOS/nixpkgs/commits/380be19fbd2d9079f677978361792cb25e8a3635
    nixpkgs.url = "github:nixos/nixpkgs/380be19fbd2d9079f677978361792cb25e8a3635";
  };

  outputs = { self, nixpkgs }: 
  let
    pkgs = import nixpkgs {
      system = "x86_64-linux";
      overlays = [ (import (misc/nix/rust-overlay)) ];
    };
    ffmpeg = pkgs.callPackage misc/nix/ffmpeg.nix {};
    tflite = pkgs.callPackage misc/nix/tflite.nix {};
    libedgetpu = pkgs.callPackage misc/nix/libedgetpu.nix {};

    rustPlatform = pkgs.makeRustPlatform {
      cargo = pkgs.rust-bin.stable."1.85.0".minimal;
      rustc = pkgs.rust-bin.stable."1.85.0".minimal;
    };

    cargoTomlContent = builtins.readFile ./Cargo.toml;
    projectMetadata = builtins.fromTOML cargoTomlContent;

  in {
    packages.x86_64-linux.sentryshot = rustPlatform.buildRustPackage {
      pname = "sentryshot";
      version = projectMetadata.workspace.package.version;
      src = ./.;

      # This is where you specify the dependencies from your shell file
      # that your application links against.
      buildInputs = [
        ffmpeg
        tflite
        libedgetpu
        pkgs.libusb1
      ];

      nativeBuildInputs = [
        pkgs.pkg-config
        pkgs.protobuf
      ];

      # This tells Nix to use your local Cargo.lock file.
      cargoLock = {
        lockFile = ./Cargo.lock;
        outputHashes = {
          "retina-0.4.11" = "sha256-BLvE4wo5DeijfADcGQczYrmLgzb0vOr6Pl+Y+ERbj5U=";
        };
      };

      # The `meta` attribute provides metadata about the package.
      meta = with pkgs.lib; {
        description = "Video Management System";
        homepage = "https://codeberg.org/SentryShot/sentryshot";
        license = licenses.gpl2Plus;
      };
    };

    packages.x86_64-linux.dockerImage = pkgs.dockerTools.buildImage {
      name = "henryouly/sentryshot";
      tag = "latest";

      contents = pkgs.buildEnv {
        name = "image-root";
        paths = [
          self.packages.x86_64-linux.sentryshot
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
        Entrypoint = [
          "${self.packages.x86_64-linux.sentryshot}/bin/sentryshot"
        ];
        Cmd = [ "" ];
        Env = [
          "LD_LIBRARY_PATH=${pkgs.lib.makeLibraryPath [
            ffmpeg
            tflite
            libedgetpu
            pkgs.libusb1
            pkgs.openh264
          ]}"
          "TZ=America/Los_Angeles"
        ];
        ExposedPorts = {
          "2020/tcp" = { };
        };
        Volumes = {
          "/configs" = { };
          "/storage" = { };
        };
      };
    };

    packages.x86_64-linux.default = self.packages.x86_64-linux.sentryshot;

    devShells."x86_64-linux".default = pkgs.mkShell {
      nativeBuildInputs = [ pkgs.rust-bin.stable."1.85.0".minimal pkgs.pkg-config pkgs.protobuf ];
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
  };
}
