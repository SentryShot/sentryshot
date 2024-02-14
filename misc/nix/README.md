```
docker build -t codeberg.org/sentryshot/sentryshot-ci:v0.2.0 -f ./ci.dockerfile .
docker push codeberg.org/sentryshot/sentryshot-ci:v0.2.0

docker build -t codeberg.org/sentryshot/sentryshot-build-x86_64:v0.2.0 -f ./build-x86_64.dockerfile .
docker push codeberg.org/sentryshot/sentryshot-build-x86_64:v0.2.0

docker build -t codeberg.org/sentryshot/sentryshot-build-aarch64:v0.2.0 -f ./build-aarch64.dockerfile .
docker push codeberg.org/sentryshot/sentryshot-build-aarch64:v0.2.0
```
