```
nix-build ./stream.nix
docker load < result
docker push codeberg.org/sentryshot/test-stream:v0.1.0
```
