# https://nix.dev/tutorials/nixos/building-and-running-docker-images
let
  # nixos 23.05
  pkgs = import (fetchTarball "https://github.com/NixOS/nixpkgs/archive/bd836ac5e5a7358dea73cb74a013ca32864ccb86.tar.gz") {};
  ffmpeg = (
    pkgs.callPackage (
      { lib, stdenv, buildPackages, removeReferencesTo, fetchgit, pkg-config, yasm }:
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
            "--disable-everything"
            "--disable-ffprobe"
            "--enable-shared"
            "--enable-protocol=file,pipe"
            "--enable-demuxer=mov"
            "--enable-decoder=h264"
            "--enable-muxer=rtsp"
            "--pkg-config=${buildPackages.pkg-config.targetPrefix}pkg-config"
            "--bindir=${placeholder "bin"}/bin"
            "--libdir=${placeholder "lib"}/lib"
            "--incdir=${placeholder "dev"}/include"
          ];

          postConfigure = let
            toStrip = lib.remove "data" outputs; # We want to keep references to the data dir.
          in
            "remove-references-to ${lib.concatStringsSep " " (map (o: "-t ${placeholder o}") toStrip)} config.h";

          nativeBuildInputs = [ removeReferencesTo pkg-config yasm ];
          buildFlags = [ "all" ];
          doCheck = false;
          outputs = [ "bin" "lib" "dev" "out" ];
          enableParallelBuilding = true;
        }
      )
    )
  {} );
in pkgs.dockerTools.buildLayeredImage {
  name = "codeberg.org/sentryshot/test-stream";
  tag = "v0.1.0";
  contents = [
    ffmpeg
    pkgs.busybox
    pkgs.mediamtx
    (pkgs.writeScriptBin "init.sh" ''
      #!${pkgs.runtimeShell}
      mediamtx &
      sleep 5
      ffmpeg -v warning -an -re -stream_loop -1 -i video.mp4 -c copy -f rtsp \
        -rtsp_transport tcp rtsp://127.0.0.1:8554/1 #>/dev/null 2>&1 </dev/null
    '')
    ./files
  ];
  config = {
    Cmd = [ "init.sh" ];
    Env = [
      "MTX_RTSPADDRESS=127.0.0.1:8554"
      "MTX_PATHS_1=rtsp://1"
      "MTX_PROTOCOLS=tcp"
      "MTX_RTMPDISABLE=yes"
      "MTX_HLSDISABLE=yes"
      "MTX_WEBRTCDISABLE=yes"
    ];
  };
}
