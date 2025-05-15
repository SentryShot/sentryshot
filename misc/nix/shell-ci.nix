let
  # glibc 2.34 nixos-22.05
  pkgs = import (fetchTarball "https://github.com/NixOS/nixpkgs/archive/380be19fbd2d9079f677978361792cb25e8a3635.tar.gz")
  { overlays = [ (import (./rust-overlay)) ];};
  ffmpeg = pkgs.callPackage ./ffmpeg.nix {};
  tflite = pkgs.callPackage ./tflite.nix {};
  libedgetpu = pkgs.callPackage ./libedgetpu.nix {};
 in pkgs.mkShell {
  nativeBuildInputs = [
    pkgs.rust-bin.stable."1.85.0".default
    pkgs.clang
    pkgs.mold
    pkgs.pkg-config
    pkgs.nodejs-18_x
    pkgs.shellcheck
  ];
  buildInputs = [ ffmpeg tflite libedgetpu pkgs.libusb1 pkgs.openh264 ];

  # Debug builds don't work without this.
  hardeningDisable = [ "fortify" ];

  LD_LIBRARY_PATH = "${ffmpeg}/lib:${tflite}/lib:${libedgetpu}/lib:${pkgs.libusb1}/lib:${pkgs.openh264}/lib";

  FFLIBS = "${ffmpeg}/lib";
  TFLITELIB = "${tflite}/lib";
  EDGETPULIB= "${libedgetpu}/lib";
  OPENH264LIB= "${pkgs.openh264}/lib";
  CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_LINKER = "clang";
  CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_RUSTFLAGS = "-C link-args=-fuse-ld=mold";
}
