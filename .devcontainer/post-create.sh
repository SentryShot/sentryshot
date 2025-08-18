#!/bin/sh
set -e

# Install nix
curl -L https://nixos.org/nix/install | sh

./misc/utils.sh dev-env-nix
