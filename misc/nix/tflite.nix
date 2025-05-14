{
  lib,
  pkgs,
  stdenv,
  fetchFromGitHub,
  fetchFromGitLab,
  fetchurl,
  fetchzip,
  cmake,
}:
let
  flatbuffers = (pkgs.callPackage (
    { fetchFromGitHub, cmake, python3}:
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
in stdenv.mkDerivation rec {
  pname = "tflite";
  version = "v2.12.1";

  src = fetchFromGitHub {
    owner = "tensorflow";
    repo = "tensorflow";
    rev = "${version}";
    sha256 = "sha256-rU7xUoF5pvpOGdJV/9mqOLkBV8ll0Oe7pRS3p5qkb0o=";
  };

  abseil_src = fetchFromGitHub {
    owner = "abseil";
    repo = "abseil-cpp";
    rev = "273292d1cfc0a94a65082ee350509af1d113344d";
    hash = "sha256-cnvLcBaznltTHJ5FSTuHhsRMmsDbJ9gyvhrBOdul288=";
  };
  eigen_src = fetchFromGitLab {
    owner = "libeigen";
    repo = "eigen";
    rev = "3460f3558e7b469efb8a225894e21929c8c77629";
    hash = "sha256-0qX7JjkroQkxJ5K442R7y1RDVoxvjWmGceqZz+8CB6A=";
  };
  farmhash_src = fetchFromGitHub {
    owner = "google";
    repo = "farmhash";
    rev = "0d859a811870d10f53a594927d0d0b97573ad06d";
    hash = "sha256-J0AhHVOvPFT2SqvQ+evFiBoVfdHthZSBXzAhUepARfA=";
  };
  fft2d_src = fetchzip {
    url = "https://storage.googleapis.com/mirror.tensorflow.org/github.com/petewarden/OouraFFT/archive/v1.0.tar.gz";
    hash = "sha256-mkG6jWuMVzCB433qk2wW/HPA9vp/LivPTDa2c0hFir4=";
  };
  gemmlowp_src = fetchFromGitHub {
    owner = "google";
    repo = "gemmlowp";
    rev = "fda83bdc38b118cc6b56753bd540caa49e570745";
    hash = "sha256-tE+w72sfudZXWyMxG6CGMqXYswve57/cpvwrketEd+k=";
  };
  neon2sse_src = fetchzip {
    url = "https://storage.googleapis.com/mirror.tensorflow.org/github.com/intel/ARM_NEON_2_x86_SSE/archive/a15b489e1222b2087007546b4912e21293ea86ff.tar.gz";
    hash = "sha256-299ZptvdTmCnIuVVBkrpf5ZTxKPwgcGUob81tEI91F0=";
  };
  cpuinfo_src = fetchFromGitHub {
    owner = "pytorch";
    repo = "cpuinfo";
    rev = "1e83a2fdd3102f65c6f1fb602c1b320486218a99";
    hash = "sha256-28cFACca+NYE8oKlP5aWXNCLeEjhWqJ6gRnFI+VxDvg=";
  };
  ruy_src = fetchFromGitHub {
    owner = "google";
    repo = "ruy";
    rev = "3286a34cc8de6149ac6844107dfdffac91531e7";
    hash = "sha256-CvoXMTMhRBQOKtAskJYmru7RUKQX9tkP62VVwvBbF/8=";
    fetchSubmodules = true;
  };
  pthreadpool_src = fetchzip {
    url = "https://github.com/Maratyszcza/pthreadpool/archive/545ebe9f225aec6dca49109516fac02e973a3de2.zip";
    hash = "sha256-sBpMElc8kUYV6EfLD+OmrZZzeN6NDdu3U4/cInAny7M=";
  };
  xnnpack_src = fetchFromGitHub {
    owner = "google";
    repo = "XNNPACK";
    rev = "659147817805d17c7be2d60bd7bbca7e780f9c82";
    hash = "sha256-+CqA/erqsg3b4xtBYU1QwDRQeuVjbkUDJuYvwUpQXYo=";
  };
  fp16_headers_src = fetchFromGitHub {
    owner = "Maratyszcza";
    repo = "FP16";
    rev = "0a92994d729ff76a58f692d3028ca1b64b145d91";
    hash = "sha256-m2d9bqZoGWzuUPGkd29MsrdscnJRtuIkLIMp3fMmtRY=";
  };
  psimd_src = fetchzip {
    url = "https://github.com/Maratyszcza/psimd/archive/072586a71b55b7f8c584153d223e95687148a900.zip";
    hash = "sha256-lV+VZi2b4SQlRYrhKx9Dxc6HlDEFz3newvcBjTekupo=";
  };
  fxdiv_src = fetchFromGitHub {
    owner = "Maratyszcza";
    repo = "FXdiv";
    rev = "63058eff77e11aa15bf531df5dd34395ec3017c8";
    hash = "sha256-LjX5kivfHbqCIA5pF9qUvswG1gjOFo3CMpX0VR+Cn38=";
  };

  patches = [
    ./tflite_patches/fetch_abseil.patch
    ./tflite_patches/fetch_eigen.patch
    ./tflite_patches/fetch_farmhash.patch
    ./tflite_patches/fetch_fft2d.patch
    ./tflite_patches/fetch_gemmlowp.patch
    ./tflite_patches/fetch_neon2sse.patch
    ./tflite_patches/fetch_cpuinfo.patch
    ./tflite_patches/fetch_ruy.patch
    ./tflite_patches/fetch_xnnpack.patch
  ];

  nativeBuildInputs = [ cmake flatbuffers ];

  processor = if stdenv.hostPlatform.isx86 then "x86_64" else "aarch64";

  postPatch = ''
    sed -i '1s/^/set(CMAKE_SYSTEM_PROCESSOR "${processor}") /' ./tensorflow/lite/CMakeLists.txt

    # Tensorflow wants write access for its own patching.
    mkdir eigen_src
    cp -r ${eigen_src}/* ./eigen_src
    chmod 777 -R ./eigen_src
  '';

  # The Bazel build enables SSE4.2 and it's 2x faster.
  NIX_CFLAGS_COMPILE = lib.optionalString stdenv.hostPlatform.isx86 "-msse4.2";

  configurePhase = ''
    cmake \
      -DCMAKE_FIND_PACKAGE_PREFER_CONFIG=ON \
      -Dabseil_SOURCE_DIR2=${abseil_src} \
      -Deigen_SOURCE_DIR2=$(pwd)/eigen_src \
      -Dfarmhash_SOURCE_DIR2=${farmhash_src} \
      -Dfft2d_SOURCE_DIR2=${fft2d_src} \
      -Dgemmlowp_SOURCE_DIR2=${gemmlowp_src} \
      -Dneon2sse_SOURCE_DIR2=${neon2sse_src} \
      -Dcpuinfo_SOURCE_DIR2=${cpuinfo_src} \
      -Druy_SOURCE_DIR2=${ruy_src} \
      -DPTHREADPOOL_SOURCE_DIR=${pthreadpool_src} \
      -DXNNPACK_SOURCE_DIR2=${xnnpack_src} \
      -DFP16_SOURCE_DIR=${fp16_headers_src} \
      -DPSIMD_SOURCE_DIR=${psimd_src} \
      -DFXDIV_SOURCE_DIR=${fxdiv_src} \
      -DTFLITE_ENABLE_RUY=ON \
      ./tensorflow/lite/c
  '';

  installPhase = ''
    mkdir -p "$out/lib"
    cp ./libtensorflowlite_c.so "$out/lib/"
  '';
}
