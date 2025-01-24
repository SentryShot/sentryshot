## unreleased

-	update retina to fix v380 pro #42

## `v0.2.21`

-	add mqtt api #44
-	detect camera clock drift
-	add back monitor groups #46

## `v0.2.20`

-	fix logdb panic #37

## `v0.2.19`

-	reduced the binary size by 14 MIB
-   make ffmpeg decode error a warning #41
-   add vod api #1
-   logdb: handle empty entries #37
-   fix date picker

## `v0.2.18`

-   improve 'no stream found' error message

## `v0.2.17`

-   improve recorder timer reliability #14
-   fix seeking in active recordings

## `v0.2.16`

-   fix panic when new api is called on running monitors #29

## `v0.2.15`

-   add api for toggling detectors
-   add /api page
-   enforce csrf tokens even when auth is disabled

## `v0.2.14`

-   fix recdb queries when day is greater than 28 #31

## `v0.2.13`

-   update retina to fix reolink badsps

## `v0.2.12`

-   tweak recorder logging

## `v0.2.11`

-   recorder: optimize disk writes
-   tflite: improve crop size error message #24

## `v0.2.10`

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
