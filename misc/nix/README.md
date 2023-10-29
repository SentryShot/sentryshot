```
docker build -t codeberg.org/sentryshot/sentryshot-ci:v0.0.1 -f ./ci.dockerfile .
docker push codeberg.org/sentryshot/sentryshot-ci:v0.0.1

docker build -t codeberg.org/sentryshot/sentryshot-build-x86_64:v0.0.1 -f ./build-x86_64.dockerfile .
docker push codeberg.org/sentryshot/sentryshot-build-x86_64:v0.0.1

docker build -t codeberg.org/sentryshot/sentryshot-build-aarch64:v0.0.1 -f ./build-aarch64.dockerfile .
docker push codeberg.org/sentryshot/sentryshot-build-aarch64:v0.0.1
```
