{
  description = "A flake for sentryshot project";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/380be19fbd2d9079f677978361792cb25e8a3635";
  };

  outputs = { self, nixpkgs }: 
  let
    pkgs = import nixpkgs {
      system = "x86_64-linux";
      overlays = [ (import (misc/nix/rust-overlay)) ];
    };
    ffmpeg = ( pkgs.callPackage misc/nix/ffmpeg.nix {} );
    tflite = pkgs.callPackage misc/nix/tflite.nix {};
    libedgetpu = pkgs.callPackage misc/nix/libedgetpu.nix {};
  in {
    packages.x86_64-linux.hello = nixpkgs.legacyPackages.x86_64-linux.hello;

    packages.x86_64-linux.default = self.packages.x86_64-linux.hello;

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
