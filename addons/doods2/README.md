## Description
This is a addon for [DOODS2](https://github.com/snowzach/doods2), a separate service that detects objects in images. It's designed to be very easy to use, run as a container and available remotely.


## Configuration

A new field in the monitor settings will appear when the DOODS addon is enabled.

#### Enable object detection

Enable for this monitor.

#### Thresholds

Individual confidence thresholds for each object that can be detected.

#### Crop

Crop frame to focus the detector and increase accuracy.

#### Mask

Mask off areas you want the detector to ignore.

#### Detector

TensorFlow model used by DOODS to detect objects.

#### Feed rate (fps)

Frames per second to send to detector, decimals are allowed.

#### Trigger duration (sec)

Recording trigger will be active for this duration in seconds.

#### Use sub stream

If sub stream should be used instead of the main stream. Only applicable if `Sub input` is set. Results in much better performance.


## Manual installation

If you use Docker compose or bundle, then DOODS2 should already be running and all you should have to do is enable the addon. If you installed OS-NVR the bare-metal way, you need to install DOODS2 manually.

Start The DOODS2 service

	sudo docker run -p 8080:8080 curid/doods2_tf-cctv:latest

Check if the service is working

	curl 127.0.0.1:8080/version

Config file will be generated at `configs/doods.json` on first start after the addon has been enabled.