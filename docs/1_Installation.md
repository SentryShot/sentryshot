# Installation

- [Docker Install](#docker-install)
- [Tarball Install](#tarball-install)
- [Build From Source](#build-from-source)

<br>

On first start you have to enable one of the authentication plugins in `configs/sentryshot.toml`

``` diff
[[plugin]]
name = "auth_none"
-enable = false
+enable = true
```

[releases](https://codeberg.org/SentryShot/sentryshot/releases)

## Docker Install


```
docker run -it \
	--env TZ=America/New_York \
	--shm-size=500m \
	-v /docker/sentryshot/configs:/app/configs \
	-v /docker/sentryshot/storage:/app/storage \
	-p 2020:2020 \
	codeberg.org/sentryshot/sentryshot:v0.2.6
```

App will be served on `http://ip:2020/live`

### Compose

```
services:
  sentryshot:
    shm_size: 500m
    image: codeberg.org/sentryshot/sentryshot:v0.2.6
    ports:
      - 2020:2020
    environment:
      - TZ=America/New_York # Timezone.
    volumes:
      - /docker/sentryshot/configs:/app/configs
      - /docker/sentryshot/storage:/app/storage
    #devices:
    #  - "/sys/bus/usb/devices/x"
```

<br>


## Tarball Install

Requires a system with `glibc 2.34+` Check with `ld -v`. `libusb-1.0` is not included.

Download a tarball from the [releases](https://codeberg.org/SentryShot/sentryshot/releases) page.

```
mkdir sentryshot
tar -xzvf sentryshot-* -C ./sentryshot/
cd sentryshot
./sentryshot --help
```

Help wanted for proper install instructions.

<br>

## Build From Source

#### Dependencies

- [rust](https://www.rust-lang.org/tools/install) 1.65+
* [libtensorflowlite_c](https://www.tensorflow.org/lite/guide/build_cmake#build_tensorflow_lite_c_library)
- [libedgetpu](https://github.com/google-coral/libedgetpu)
- `libavutil-dev`
- `libavcodec-dev`
- `libusb-1.0-0-dev`
- `git`
- `pkg-config`



```
# Clone the repository.
git clone --branch master https://codeberg.org/SentryShot/sentryshot.git
cd sentryshot

# Build 
./misc/utils build-target x86_64

# Make a tarball.
tar -czvf "./build/sentryshot-x86_64.tar.gz" -C "./build/x86_64"
```
[Tarball Install](#tarball-install)