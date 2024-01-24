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

