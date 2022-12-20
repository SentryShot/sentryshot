# Configuration

- [General](#general)
	- [Disk space](#disk-space)
	- [Theme](#theme)
	
- [Monitors](#monitors)
	- [ID](#id)
	- [Name](#name)
	- [Enable](#enable)
	- [Url](#url)
	- [Hardware Acceleration](#hardware-acceleration)
	- [Video encoder](#video-encoder)
	- [Audio encoder](#audio-encoder)
	- [Always record](#always-record)
	- [Video length](#video-length)
	- [Timestamp offset](#timestamp-offset)
	- [Log level](#log-level)

- [Users](#users)
- [Addons](#addons)
- [Environment](#environment)

<br>

## General
Settings that don't belong anywhere else.

#### Max disk usage
Maximum allowed storage space in GigaBytes. Recordings are delete automatically before this value is exceeded. Please open an issue if the disk usage ever exceed this value.

#### Theme
UI theme

<br>

## Monitors

### ID
Monitor identifier. The monitors recordings are tied to this id.

### Name
Arbitrary display name, can probably be any ASCII character.

### Enable
Enable or Disable the monitor.

### Input options

`-rtsp_transport tcp`: Force FFmpeg to use TCP instead of UDP.


### Main input
Main camera feed, full resolution. Used when recording.

### Sub input
If your camera support a sub stream of lower resolution. Both inputs can be viewed from the live page.

<br>


### Hardware acceleration
To view supported hardware accelerators.

	ffmpeg -hwaccels

##### Examples:
CUDA

	cuda

CUDA with device specified

	cuda -hwaccel_device 0

<br>

### Video encoder
Select video encoder for converting the camera input to h264. 

To view available h264 encoders.

	ffmpeg -encoders | grep h264

##### Options
copy: Pass feed directly from the input. Does not transcode. Requires h264 input.

libx264*: Transcode input to h264. Usually not recommended. A slower preset will provide better compression at the cost of processing power.

custom: Any value, for example`h264_nvenc` in the case of hardware acceleration.

<br>

### Audio encoder
To view available encoders.

	ffmpeg -encoders

##### Options
none: Do not save audio.

copy: Pass feed directly from the input.

aac: Transcode input to AAC.

custom: Any value.

<br>

### Always record
Always record.

<br>

### Video Length
Maximum video length in minutes.

<br>

### Timestamp offset
Remove this amount in milliseconds from the timestamp. 

For example if the recording is 2 seconds behind the timestamp set this to 2000

<br>

### Log level
ffmpeg log level.

##### Options
quiet: Show nothing at all; be silent.

fatal: Only show fatal errors. These are errors after which the process absolutely cannot continue. 

error: Show all errors, including ones which can be recovered from.

warning: Show all warnings and errors. Any message related to possibly incorrect or unexpected events will be shown.

info: Show informative messages during processing. This is in addition to warnings and errors.

debug: Show everything, including debugging information.

<br>

## Users
##### Fields: 

Username: Name of user.

Admin: If user has admin privileges or not.

New password: Set initial or change password.

Repeat password: Confirm password.


<br>


## Environment 

Environment is configured in `env.yaml` default location `/home/_nvr/os-nvr/configs/env.yaml`
