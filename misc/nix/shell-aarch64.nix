let
  # glibc 2.34 nixos-22.05
  pkgs = (import (fetchTarball "https://github.com/NixOS/nixpkgs/archive/380be19fbd2d9079f677978361792cb25e8a3635.tar.gz")
    {
      crossSystem = "aarch64-unknown-linux-gnu";
      overlays = [ (import (./rust-overlay)) ];
    }).__splicedPackages;
  ffmpeg = pkgs.callPackage ./ffmpeg.nix {};
  tflite = pkgs.callPackage ./tflite.nix {};
  libedgetpu = pkgs.callPackage ./libedgetpu.nix {};
in pkgs.mkShell {
  nativeBuildInputs = [ pkgs.rust-bin.stable."1.85.0".minimal pkgs.pkg-config pkgs.curl ];
  buildInputs = [ ffmpeg tflite libedgetpu pkgs.libusb1 pkgs.openh264 pkgs.protobuf ];

  LD_LIBRARY_PATH = "${ffmpeg}/lib:${tflite}/lib:${libedgetpu}/lib:${pkgs.libusb1}/lib:${pkgs.openh264}/lib";

  FFLIBS = "${ffmpeg}/lib";
  TFLITELIB = "${tflite}/lib";
  EDGETPULIB= "${libedgetpu}/lib";
  OPENH264LIB= "${pkgs.openh264}/lib";
  CARGO_BUILD_TARGET = "aarch64-unknown-linux-gnu";
  CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER = "${pkgs.stdenv.cc.targetPrefix}cc";
  CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_RUSTFLAGS = "-C link-args=-Wl,-rpath,$ORIGIN/libs:$ORIGIN/../libs";
}
