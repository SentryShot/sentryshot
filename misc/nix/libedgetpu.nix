{ pkgs, stdenv, fetchFromGitHub, libusb1, xxd }:
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
