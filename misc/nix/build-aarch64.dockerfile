FROM nixos/nix:2.17.0

#RUN mkdir /libs
#COPY aarch64-tflite/out/libtensorflowlite_c.so /libs/
#ENV LIBRARY_PATH=/libs
#ENV LD_LIBRARY_PATH=/libs
#ENV TFLITELIB=/libs/libtensorflow_c.so

COPY shell-aarch64.nix /shell.nix
COPY ffmpeg.nix /ffmpeg.nix
COPY libedgetpu.nix /libedgetpu.nix
COPY libedgetpu_makefile.patch /libedgetpu_makefile.patch
COPY rust-overlay /rust-overlay
COPY tflite.nix /tflite.nix
COPY tflite_patches /tflite_patches

ENV NIXPKGS_ALLOW_UNSUPPORTED_SYSTEM=1
RUN until nix-shell /shell.nix --command "true"; do sleep 300; done
#RUN nix-shell /shell.nix --command "true"


ENTRYPOINT nix-shell
