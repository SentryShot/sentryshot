## Description
This is a addon for [DOODS2](https://github.com/snowzach/doods2), a service that detects objects in images. It's designed to be very easy to use, run as a container and available remotely.

## Installation

##### Note: This is installed by default in Docker.

Install and start the DOODS2 server.

Check if server is working.

	curl 127.0.0.1:8080/version

Uncomment `- nvr/addons/doods2` in `configs/env.yaml`

Restart OS-NVR

	sudo systemctl restart nvr

Config file will be generated at `configs/doods.json`

DOODS port can be changed here. Default:`8080`


## Configuration

#### DOODS enable

Enable for this monitor.

#### DOODS detector

Detector model used by DOODS to detect objects.

#### DOODS thresholds

Individual confidence thresholds for each type of object that can be detected.

#### DOODS crop

Crop frame to focus the detector and increase accuracy.

#### DOODS mask

Mask areas you want the detector to ignore.

#### DOODS feed rate

Frames per second to send to detector, decimals are allowed.

#### DOODS trigger duration

Recording trigger will be active for this duration in seconds.

#### DOODS use substream

If sub stream should be used instead of the main stream. Only applicable if `Sub input` is set. Results in much better performance.