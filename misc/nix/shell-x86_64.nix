let
  # glibc 2.34 nixos-22.05
  pkgs = import (fetchTarball "https://github.com/NixOS/nixpkgs/archive/380be19fbd2d9079f677978361792cb25e8a3635.tar.gz")
  { overlays = [ (import (fetchTarball "https://github.com/oxalica/rust-overlay/archive/c707d9606ff9acea7f9508f5ad8218e36a96b126.tar.gz")) ];};
  ffmpeg = ( pkgs.callPackage ./ffmpeg.nix {} );
  bazel_5 = ( pkgs.callPackage ./bazel_5.nix { buildJdk = pkgs.jdk11_headless; runJdk = pkgs.jdk11_headless; });
  tflite = ( pkgs.callPackage ./tflite.nix { bazel_5 = bazel_5; });
  libedgetpu = pkgs.callPackage ./libedgetpu.nix {};
in pkgs.mkShell {
  nativeBuildInputs = [ pkgs.rust-bin.stable."1.65.0".minimal pkgs.pkg-config ];
  buildInputs = [ ffmpeg tflite libedgetpu pkgs.libusb1 ];

  LD_LIBRARY_PATH = "${ffmpeg}/lib:${tflite}/lib:${libedgetpu}/lib:${pkgs.libusb1}/lib";

  FFLIBS = "${ffmpeg}/lib";
  TFLITELIB = "${tflite}/lib";
  EDGETPULIB= "${libedgetpu}/lib";
  CARGO_BUILD_TARGET = "x86_64-unknown-linux-gnu";
  CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_LINKER = "${pkgs.stdenv.cc.targetPrefix}cc";
  CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_RUSTFLAGS = "-C link-args=-Wl,-rpath,$ORIGIN/libs:$ORIGIN/../libs";
}
