{ stdenv, buildBazelPackage, buildPackages, fetchFromGitHub, lib, bazel_5 }:
let
  buildPlatform = stdenv.buildPlatform;
  hostPlatform = stdenv.hostPlatform;
  pythonEnv = buildPackages.python3.withPackages (ps: [ ps.numpy ]);
  bazelDepsSha256ByBuildAndHost = {
    # The checksums are unreliable.
    x86_64-linux = {
      #x86_64-linux = "sha256-QlHy5FcgAfdbST3WLShI3wL416n2T1u8a0ceGq9f638=";
      x86_64-linux = "sha256-u3a3szxS2o+jRLqO94ks2h6xzpiAdlsaWRQSfQH9S2c";
      aarch64-linux = "sha256-sOIYpp98wJRz3RGvPasyNEJ05W29913Lsm+oi/aq/Ag=";
    };
    aarch64-linux = {
      aarch64-linux = "sha256-NoQmTDiTzLkEe6uz5Jy250ZHbKeLFp75UuTh/rjt3yM=";
    };
  };
  bazelHostConfigName.aarch64-linux = "elinux_aarch64";
  bazelDepsSha256ByHost =
    bazelDepsSha256ByBuildAndHost.${buildPlatform.system} or
      (throw "unsupported build system ${buildPlatform.system}");
  bazelDepsSha256 = bazelDepsSha256ByHost.${hostPlatform.system} or
      (throw "unsupported host system ${hostPlatform.system} with build system ${buildPlatform.system}");
in
buildBazelPackage rec {
  name = "tflite";
  version = "2.12.1";

  src = fetchFromGitHub {
    owner = "tensorflow";
    repo = "tensorflow";
    rev = "v${version}";
    hash = "sha256-Rq5pAVmxlWBVnph20fkAwbfy+iuBNlfFy14poDPd5h0=";
  };

  bazel = bazel_5;

  nativeBuildInputs = [ pythonEnv buildPackages.perl ];

  bazelTarget = "//tensorflow/lite/c:tensorflowlite_c";

  bazelFlags = [
    "--config=opt"
  ] ++ lib.optionals (hostPlatform.system != buildPlatform.system) [
    "--config=${bazelHostConfigName.${hostPlatform.system}}"
  ];

  bazelBuildFlags = [ "--cxxopt=--std=c++17" ];

  buildAttrs = {
    installPhase = ''
      mkdir -p $out/lib
      cp ./bazel-bin/tensorflow/lite/c/libtensorflowlite_c.so $out/lib/
    '';
  };

  fetchAttrs.sha256 = bazelDepsSha256;

  PYTHON_BIN_PATH = pythonEnv.interpreter;

  dontAddBazelOpts = true;
  removeRulesCC = false;

  postPatch = "rm .bazelversion";
  preConfigure = "patchShebangs configure";

  dontAddPrefix = true;
  configurePlatforms = [];
}
