FROM nixos/nix:2.17.0

RUN mkdir /libs
COPY aarch64-tflite/out/libtensorflowlite_c.so /libs/
ENV LIBRARY_PATH=/libs
ENV LD_LIBRARY_PATH=/libs
ENV TFLITELIB=/libs/libtensorflow_c.so

COPY shell-aarch64.nix /shell.nix
COPY bazel_5.nix /bazel_5.nix
COPY ffmpeg.nix /ffmpeg.nix
COPY libedgetpu.nix /libedgetpu.nix
COPY patches /patches

ENV NIXPKGS_ALLOW_UNSUPPORTED_SYSTEM=1
#RUN until nix-shell /shell.nix --command "true"; do sleep 300; done
RUN nix-shell /shell.nix --command "true"


ENTRYPOINT nix-shell
