FROM nixos/nix:2.17.0

COPY shell-ci.nix /shell.nix
COPY ffmpeg.nix /ffmpeg.nix
COPY libedgetpu.nix /libedgetpu.nix
COPY libedgetpu_makefile.patch /libedgetpu_makefile.patch
COPY rust-overlay /rust-overlay
COPY tflite.nix /tflite.nix
COPY tflite_patches /tflite_patches

RUN nix-shell /shell.nix --command "true"

ENTRYPOINT nix-shell
