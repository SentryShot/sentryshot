let
  # glibc 2.34 nixos-22.05
  pkgs = (import (fetchTarball "https://github.com/NixOS/nixpkgs/archive/380be19fbd2d9079f677978361792cb25e8a3635.tar.gz")
    {
      crossSystem = "aarch64-unknown-linux-gnu";
      overlays = [ (import (fetchTarball "https://github.com/oxalica/rust-overlay/archive/e6679d2ff9136d00b3a7168d2bf1dff9e84c5758.tar.gz")) ];
    }).__splicedPackages;
  ffmpeg = ( pkgs.callPackage ./ffmpeg.nix {} );
  libedgetpu = pkgs.callPackage ./libedgetpu.nix {};
in pkgs.mkShell {
  nativeBuildInputs = [ pkgs.rust-bin.stable."1.75.0".minimal pkgs.pkg-config pkgs.curl ];
  buildInputs = [ ffmpeg libedgetpu pkgs.libusb1 pkgs.openh264 ];

  LD_LIBRARY_PATH = "${ffmpeg}/lib:${libedgetpu}/lib:${pkgs.libusb1}/lib:${pkgs.openh264}/lib";

  FFLIBS = "${ffmpeg}/lib";
  EDGETPULIB= "${libedgetpu}/lib";
  OPENH264LIB= "${pkgs.openh264}/lib";
  CARGO_BUILD_TARGET = "aarch64-unknown-linux-gnu";
  CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER = "${pkgs.stdenv.cc.targetPrefix}cc";
  CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_RUSTFLAGS = "-C link-args=-Wl,-rpath,$ORIGIN/libs:$ORIGIN/../libs";
}
