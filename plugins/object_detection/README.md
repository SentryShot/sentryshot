## Description
Plugin for [TFlite](https://www.tensorflow.org/lite) object detection.

## Configuration

A new field in the monitor settings will appear when the DOODS addon is enabled.

#### Enable object detection

Enable for this monitor.

#### Thresholds

Individual confidence thresholds for each object that can be detected. A threshold of 100 means that it must be 100% confident about the object before a event is triggered. 50 is a good starting point.

#### Crop

Crop frame to focus the detector and increase accuracy.

#### Mask

Mask off areas you want the detector to ignore. The dark marked area will be ignored.

#### Detector

TensorFlow model used by DOODS to detect objects.

#### Feed rate (fps)

Frames per second to send to detector, decimals are allowed.

#### Trigger duration (sec)

The number of seconds the recorder will be active for after a object is detected.

#### Use sub stream

If sub stream should be used instead of the main stream. Only applicable if `Sub input` is set. Results in much better performance.