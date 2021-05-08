# Configuration

* [General](#general)
	- [Disk space](#disk-space)
	- [Theme](#theme)
	
* [Monitors](#monitors)
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

* [Users](#users)
* [Addons](#addons)
* [Environment](#environment)

<br>

## General
Settings that don't belong anywhere else.

#### Disk space
Maximum allowed storage space in Gigabytes.

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

### Url
Path to video input 

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

To view available encoders.

	ffmpeg -encoders | grep h264

##### Options
copy: Pass feed directly from the input. Does not transcode. Requires h264 input.

libx264: Transcode input to h264. Will use allot of CPU, usually not recommended.

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

## Addons

Addons are configured in `addons.conf` default location `/home/_nvr/home/configs/addons.conf`

<br>

## Environment 

Environment is configured in `env.json` default location `/home/_nvr/home/configs/env.json`

##### Fields:

port: default `2020`

goBin: Path to golang binary. default `/usr/bin/go`

ffmpegBin: Path to ffmpeg binary. default `/usr/bin/ffmpeg`

shmDir: Shared memory directory, used to store tempoary files. default `/dev/shm/nvr`

homeDir: Project home. default: `/home/_nvr/nvr`

storageDir: Directory where recordings will be stored. default `/home/_nvr/nvr/storage`