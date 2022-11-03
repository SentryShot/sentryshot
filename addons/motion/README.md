## Description
This is a basic motion detection addon. Motion detection is inherently unreliable, there will be situations where it cannot be used reliably.

## Configuration

A new field in the monitor settings will appear when the motion addon is enabled.

#### Enable motion detection

Enable for this monitor.

#### Feed rate (fps)

Frames per second to send to detector, decimals are allowed.

#### Frame scale

Downscale frames to reduce CPU load. Sub-stream is used if available. "quarter" will divide the width and height by 4.

#### Trigger duration (sec)

The number of seconds the recorder will be active for when motion is detected.

## Zone options

#### Zone selector

Select a zone to configure. Use `+` and `-` to add and remove zones.

#### Sensitivity

Sensitivity is the minimum percent color change in a pixel for it to be counted as active.

#### Threshold Min-Max

Threshold is the percentage of active pixels within the area required to trigger a event.

#### Preview

Preview this zone in the UI.

#### Area

Define the area for this zone.