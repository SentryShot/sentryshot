FROM nixos/nix:2.17.0

COPY shell-ci.nix /shell.nix
COPY patches /patches
COPY src-deps.json /src-deps.json

RUN until nix-shell /shell.nix --command "true"; do sleep 300; done

ENTRYPOINT nix-shell
