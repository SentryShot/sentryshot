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
  libedgetpu = (
    pkgs.callPackage (
      { stdenv, lib, fetchFromGitHub, libusb1, xxd }:
      let
        flatbuffers = (pkgs.callPackage (
          { lib, stdenv, fetchFromGitHub, cmake, python3}:
          stdenv.mkDerivation {
            pname = "flatbuffers";
            version = "7d6d99c6befa635780a4e944d37ebfd58e68a108";
            NIX_CXXSTDLIB_COMPILE = "-std=c++17";
            src = fetchFromGitHub {
              owner = "google";
              repo = "flatbuffers";
              rev = "v2.0.6";
              hash = "sha256-0bJ0n/5yzj6lHXLKJzHUS0Bnlmys+X7pY/3LGapVh6k=";
            };
            nativeBuildInputs = [ cmake python3 ];
            meta = { mainProgram = "flatc"; };
          }
        ){});
        abseil-cpp = (pkgs.callPackage (
          { lib, stdenv, fetchFromGitHub, cmake, gtest }:
          stdenv.mkDerivation (finalAttrs: {
            pname = "abseil-cpp";
            version = "20230125.3";

            src = fetchFromGitHub {
              owner = "abseil";
              repo = "abseil-cpp";
              rev = "273292d1cfc0a94a65082ee350509af1d113344d";
              hash = "sha256-cnvLcBaznltTHJ5FSTuHhsRMmsDbJ9gyvhrBOdul288=";
            };
            cmakeFlags = [
              "-DABSL_BUILD_TEST_HELPERS=ON"
              "-DABSL_USE_EXTERNAL_GOOGLETEST=ON"
              "-DBUILD_SHARED_LIBS=ON"
            ];
            strictDeps = true;
            nativeBuildInputs = [ cmake ];
            NIX_CXXSTDLIB_COMPILE = "-std=c++17";
            buildInputs = [ gtest ];
          })
        ){});
      in stdenv.mkDerivation rec {
        pname = "libedgetpu";
        version = "grouper";

        src = fetchFromGitHub {
          owner = "google-coral";
          repo = pname;
          rev = "release-${version}";
          sha256 = "sha256-73hwItimf88Iqnb40lk4ul/PzmCNIfdt6Afi+xjNiBE=";
        };
        patches = ./patches/libedgetpu_makefile.patch;

        makeFlags = ["-f" "makefile_build/Makefile" "libedgetpu"];

        buildInputs = [ libusb1 abseil-cpp flatbuffers ];

        nativeBuildInputs = [ xxd flatbuffers ];

        NIX_CXXSTDLIB_COMPILE = "-std=c++17";

        TFROOT = "${fetchFromGitHub {
          owner = "tensorflow";
          repo = "tensorflow";
          rev = "v2.12.1";
          sha256 = "sha256-rU7xUoF5pvpOGdJV/9mqOLkBV8ll0Oe7pRS3p5qkb0o=";
        }}";

        enableParallelBuilding = false;

        installPhase = ''
          mkdir -p $out/lib
          cp out/direct/k8/libedgetpu.so.1.0 $out/lib
          ln -s $out/lib/libedgetpu.so.1.0 $out/lib/libedgetpu.so.1
          ln -s $out/lib/libedgetpu.so.1.0 $out/lib/libedgetpu.so
          mkdir -p $out/lib/udev/rules.d
          cp debian/edgetpu-accelerator.rules $out/lib/udev/rules.d/99-edgetpu-accelerator.rules
        '';
      }
    )
  {} );
in pkgs.mkShell {
  nativeBuildInputs = [ pkgs.rust-bin.stable."1.65.0".minimal pkgs.pkg-config pkgs.curl ];
  buildInputs = [ ffmpeg libedgetpu ];

  LD_LIBRARY_PATH = "${ffmpeg}/lib:${libedgetpu}/lib";

  FFLIBS = "${ffmpeg}/lib";
  EDGETPULIB= "${libedgetpu}/lib";
  CARGO_BUILD_TARGET = "aarch64-unknown-linux-gnu";
  CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER = "${pkgs.stdenv.cc.targetPrefix}cc";
  CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_RUSTFLAGS = "-C link-args=-Wl,-rpath,$ORIGIN/libs:$ORIGIN/../libs";
}
