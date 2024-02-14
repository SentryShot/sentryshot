FROM nixos/nix:2.17.0

COPY shell-ci.nix /shell.nix
COPY bazel_5.nix /bazel_5.nix
COPY ffmpeg.nix /ffmpeg.nix
COPY libedgetpu.nix /libedgetpu.nix
COPY tflite.nix /tflite.nix
COPY patches /patches
COPY src-deps.json /src-deps.json

RUN until nix-shell /shell.nix --command "true"; do sleep 300; done

ENTRYPOINT nix-shell
