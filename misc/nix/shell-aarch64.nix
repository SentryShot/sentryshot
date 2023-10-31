let
  # glibc 2.34 nixos-22.05
  pkgs = (import (fetchTarball "https://github.com/NixOS/nixpkgs/archive/380be19fbd2d9079f677978361792cb25e8a3635.tar.gz")
    {
      crossSystem = "aarch64-unknown-linux-gnu";
      overlays = [ (import (fetchTarball "https://github.com/oxalica/rust-overlay/archive/c707d9606ff9acea7f9508f5ad8218e36a96b126.tar.gz")) ];
    }).__splicedPackages;
  ffmpeg = (
    pkgs.callPackage (
      { lib, stdenv, buildPackages, removeReferencesTo, fetchgit, pkg-config, yasm }:
      let inherit (lib) optionals; in

      stdenv.mkDerivation (
        rec {
          pname = "ffmpeg";
          version = "6.0";

          src = fetchgit {
            url = "https://git.ffmpeg.org/ffmpeg.git";
            rev = "n${version}";
            sha256 = "sha256-RVbgsafIbeUUNXmUbDQ03ZN42oaUo0njqROo7KOQgv0=";
          };

          configurePlatforms = [];
          setOutputFlags = false; # Only accepts some of them.
          configureFlags = [
            "--disable-all"
            "--enable-shared"
            "--enable-avcodec"
            "--enable-avutil"
            "--enable-decoder=h264"
            "--target_os=${stdenv.hostPlatform.parsed.kernel.name}"
            "--arch=${stdenv.hostPlatform.parsed.cpu.name}"
            "--pkg-config=${buildPackages.pkg-config.targetPrefix}pkg-config"
            "--datadir=${placeholder "data"}/share/ffmpeg"
            "--libdir=${placeholder "lib"}/lib"
            "--incdir=${placeholder "dev"}/include"
          ] ++ optionals (stdenv.hostPlatform != stdenv.buildPlatform) [
            "--cross-prefix=${stdenv.cc.targetPrefix}"
            "--enable-cross-compile"
            "--host-cc=${buildPackages.stdenv.cc}/bin/cc"
          ] ++ optionals stdenv.cc.isClang [
            "--cc=clang"
          ];

          postConfigure = let
            toStrip = lib.remove "data" outputs; # We want to keep references to the data dir.
          in
            "remove-references-to ${lib.concatStringsSep " " (map (o: "-t ${placeholder o}") toStrip)} config.h";

          nativeBuildInputs = [ removeReferencesTo pkg-config yasm ];
          buildFlags = [ "all" ];
          doCheck = false;
          outputs = [ "lib" "dev" "data" "out" ];
        }
      )
    )
  {} );
in pkgs.mkShell {
  nativeBuildInputs = [ pkgs.rust-bin.stable."1.65.0".minimal pkgs.pkg-config pkgs.curl ];
  buildInputs = [ ffmpeg ];

  FFLIBS = "${ffmpeg}/lib";
  CARGO_BUILD_TARGET = "aarch64-unknown-linux-gnu";
  CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER = "${pkgs.stdenv.cc.targetPrefix}cc";
  CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_RUSTFLAGS = "-C link-args=-Wl,-rpath,$ORIGIN/libs:$ORIGIN/../libs";
}
