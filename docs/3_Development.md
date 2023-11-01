### Index

- [Development Environment](#development-environment)
- [Test Stream](#test-stream)
- [Program Map](#program-map)
- [The Plugin System](#the-plugin-system)

<br>

## Development Environment

Use this command to run the full CI suite after setting up your environment

    ./misc/utils.sh ci-fix



### Nix shell

Use a [nix shell](https://nix.dev/tutorials/first-steps/ad-hoc-shell-environments) with all the build tools installed. Will use 5GB disk space. [Install Nix](https://nixos.org/download#download-nix)

    ./misc/utils.sh dev-env-nix


### Docker

Enter a nix shell packaged inside a OCI image. This is the same image that the CI pipeline uses.

	./misc/utils.sh dev-env-docker 

### Manual

You can install everything manually instead. Most things don't require a very specific version, but they may not exist in your package manager.

* [rust 1.65+](https://www.rust-lang.org/tools/install)
* [node 18+](https://nodejs.org)
* [libavutil+libavcodec](https://ffmpeg.org)
* [libtensorflowlite_c](https://www.tensorflow.org/lite/guide/build_cmake#build_tensorflow_lite_c_library)
* [shellcheck](https://www.shellcheck.net)
* pkg-config

<br>

## Test Stream

```
docker run -it --network=host codeberg.org/sentryshot/test-stream:v0.1.0
ffplay -rtsp_transport tcp rtsp://127.0.0.1:8554/1
```

<br>

## Program Map

```
TODO:
```



<br>

## The Plugin System

Each plugin is a `dylib` shared library. Enabled plugins are loaded at runtime with [libloading](https://github.com/nagisa/rust_libloading), unloading or reloading is not supported.

https://github.com/luojia65/plugin-system-example

### Tokio limitations

Tokio stores internal state in a global variable that isn't shared across shared libraries. Calling Tokio functions from a plugin must be done using a injected runtime handle, `self.rt_handle.spawn()` or by adding `let _enter = self.rt_handle.enter()` just before the function.