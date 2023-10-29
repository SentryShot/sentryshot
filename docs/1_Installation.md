# Installation

- [Docker Install](#docker-install)
- [Tarball Install](#tarball-install)
- [Build From Source](#build-from-source)

<br>

## Docker Instalaaaa:sdada

```
docker run -it \
	--env TZ=America/New_York \
	--shm-size=500m \
	-v /docker/sentryshot/configs:/app/configs \
	-v /docker/sentryshot/storage:/app/storage \
	-p 2020:2020 \
	todo:latest
```

App will be served on `https://ip:2020`

### Compose

```
todo
```

<br>


## Tarball Install

TODO

<br>

## Build From Source

#### Dependencies

- [rust](https://www.rust-lang.org/tools/install) 1.65+
- [ffmpeg](https://ffmpeg.org/download.html) 
- [tflite](https://www.tensorflow.org/lite/guide/build_cmake#build_tensorflow_lite_c_library)
- git


```
# Clone the repository.
git clone --branch master https://codeberg.org/SentryShot/sentryshot.git
cd sentryshot

# Build 
./misc/utils build-target x86_64
```

TODO