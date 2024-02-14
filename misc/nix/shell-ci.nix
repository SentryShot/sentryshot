let
  # glibc 2.34 nixos-22.05
  pkgs = import (fetchTarball "https://github.com/NixOS/nixpkgs/archive/380be19fbd2d9079f677978361792cb25e8a3635.tar.gz")
  { overlays = [ (import (fetchTarball "https://github.com/oxalica/rust-overlay/archive/e6679d2ff9136d00b3a7168d2bf1dff9e84c5758.tar.gz")) ];};
  ffmpeg = pkgs.callPackage ./ffmpeg.nix {};
  bazel_5 = pkgs.callPackage ./bazel_5.nix { buildJdk = pkgs.jdk11_headless; runJdk = pkgs.jdk11_headless; };
  tflite = pkgs.callPackage ./tflite.nix { bazel_5 = bazel_5; };
  libedgetpu = pkgs.callPackage ./libedgetpu.nix {};
 in pkgs.mkShell {
  nativeBuildInputs = [
    pkgs.rust-bin.stable."1.75.0".default
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
  #CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_RUSTFLAGS = "-C link-args=-fuse-ld=$mold,-Wl,-rpath,$ORIGIN/libs:$ORIGIN/../libs";
}
