## unreleased

-   tflite: fix sub-stream toggle

## `v0.2.9`

-   fix initial thumbnail generation #23
-   make active recordings viewable
-   implement delete button

## `v0.2.8`

-   motion: fix saving zone sensitivity #22

## `v0.2.7`

-   fix live page fullscreen #20

## `v0.2.6`

-   limit fullscreen to window size #20

## `v0.2.5`

-   fix download filename #19

## `v0.2.4`

-   cli: fix --version

## `v0.2.3`

-   proper fix for broken recorder recovery #14

## `v0.2.2`

-   hotfix broken recorder recovery #14

## `v0.2.1`

-   fix extra live feed delay with multiple clients

## `v0.2.0`

-   BREAKING: update toolchain to Rust 1.75
-   fix default tflite crop size
-   add tflite edgetpu support
-   BREAKING: the tflite plugin now depend on `libedgetpu` and `libusb-1.0-0-dev`
-   BREAKING: empty and missing CSV queries now have the same behavior

## `v0.1.3`

-   use webpki root certificates instead of native
-   fix logs page race condition
-   add log file read buffering
-   add central stream decoder

## `v0.1.2`

-   zone editor redesign #4
-   add jsdoc type checking
-   add typescript as a linter
-   use `.js` as the javascript file extension
-   replace clap with pico-args

## `v0.1.1`

-   fix date picker

## `v0.1.0`

-   RUST REWIRTE [9949f50ef0](https://codeberg.org/SentryShot/sentryshot/commit/9949f50ef058501d0c69b54a59c447498d80f119)
