let
  # glibc 2.34 nixos-22.05
  pkgs = (import (fetchTarball "https://github.com/NixOS/nixpkgs/archive/380be19fbd2d9079f677978361792cb25e8a3635.tar.gz")
    {
      crossSystem = "aarch64-unknown-linux-gnu";
      overlays = [ (import (fetchTarball "https://github.com/oxalica/rust-overlay/archive/c707d9606ff9acea7f9508f5ad8218e36a96b126.tar.gz")) ];
    }).__splicedPackages;
  ffmpeg = ( pkgs.callPackage ./ffmpeg.nix {} );
  libedgetpu = pkgs.callPackage ./libedgetpu.nix {};
in pkgs.mkShell {
  nativeBuildInputs = [ pkgs.rust-bin.stable."1.65.0".minimal pkgs.pkg-config pkgs.curl ];
  buildInputs = [ ffmpeg libedgetpu pkgs.libusb1 ];

  LD_LIBRARY_PATH = "${ffmpeg}/lib:${libedgetpu}/lib:${pkgs.libusb1}/lib";

  FFLIBS = "${ffmpeg}/lib";
  EDGETPULIB= "${libedgetpu}/lib";
  CARGO_BUILD_TARGET = "aarch64-unknown-linux-gnu";
  CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER = "${pkgs.stdenv.cc.targetPrefix}cc";
  CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_RUSTFLAGS = "-C link-args=-Wl,-rpath,$ORIGIN/libs:$ORIGIN/../libs";
}
