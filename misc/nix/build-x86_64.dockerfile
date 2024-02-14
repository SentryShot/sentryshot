FROM nixos/nix:2.17.0

COPY shell-x86_64.nix /shell.nix
COPY bazel_5.nix /bazel_5.nix
COPY ffmpeg.nix /ffmpeg.nix
COPY libedgetpu.nix /libedgetpu.nix
COPY tflite.nix /tflite.nix
COPY src-deps.json /src-deps.json
COPY patches /patches

RUN until nix-shell /shell.nix --command "true"; do sleep 300; done

ENTRYPOINT nix-shell
