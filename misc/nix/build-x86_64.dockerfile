FROM nixos/nix:2.17.0

COPY shell-x86_64.nix /shell.nix
COPY src-deps.json /src-deps.json
COPY patches /patches

RUN until nix-shell /shell.nix --command "true"; do sleep 300; done

ENTRYPOINT nix-shell
