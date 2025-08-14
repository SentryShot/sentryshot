# Design Doc: Integrating a New Object Detection Plugin

This document outlines the process for creating a new object detection plugin for SentryShot, using the existing `object_detection` plugin as a reference. The goal is to provide a clear and detailed guide for developers to obtain video frames from the system and process them with their own detection logic.

## 1. Overview

The SentryShot plugin system allows for extending its functionality. An object detection plugin's primary responsibility is to:
1.  Receive video frames for a specific monitor.
2.  Process these frames to detect objects.
3.  Trigger events when objects are detected.

The core of the object detection logic resides within the `on_monitor_start` function of the `Plugin` trait implementation. This function is the entry point for your plugin's interaction with a monitor's video feed.

## 2. Plugin Initialization and Structure

Every plugin must implement the `Plugin` trait. The lifecycle of a plugin begins when it's loaded by the main application.

### 2.1. Loading the Plugin

The `load` function is the entry point for your plugin. It's responsible for creating an instance of your plugin's main struct.

```rust
// In plugins/your_plugin/your_plugin.rs

#[unsafe(no_mangle)]
pub extern "Rust" fn load(app: &dyn Application) -> Arc<dyn Plugin> {
    app.rt_handle().block_on(async {
        Arc::new(
            YourObjectDetectionPlugin::new(
                app.rt_handle(),
                app.shutdown_complete_tx(),
                app.logger(),
                app.env(),
                app.monitor_manager(),
            )
            .await,
        )
    })
}
```

Your `YourObjectDetectionPlugin::new` function should initialize any required components, such as a `DetectorManager` that handles loading models and detector instances.

### 2.2. The `on_monitor_start` Hook

This is the most critical function for an object detection plugin. It's called for each active monitor when the system starts.

```rust
// In plugins/your_plugin/your_plugin.rs

#[async_trait]
impl Plugin for YourObjectDetectionPlugin {
    async fn on_monitor_start(&self, token: CancellationToken, monitor: ArcMonitor) {
        let msg_logger = Arc::new(YourMsgLogger {
            logger: self.logger.clone(),
            monitor_id: monitor.config().id().to_owned(),
        });

        if let Err(e) = self.start(&token, msg_logger.clone(), monitor).await {
            msg_logger.log(LogLevel::Error, &format!("start: {e}"));
        };
    }
    // ... other trait methods
}
```

The `on_monitor_start` function receives a `CancellationToken` to gracefully shut down and an `ArcMonitor` instance, which provides access to the monitor's configuration and video streams.

## 3. Obtaining Video Frames

The process of getting video frames involves these steps:
1.  Parse the monitor's configuration to see if your plugin is enabled and to get any specific settings.
2.  Choose the appropriate video stream (main or sub-stream).
3.  Subscribe to the decoded video feed.
4.  Receive and process frames in a loop.

### 3.1. Parsing Configuration

First, you need to parse the monitor's configuration to determine if your plugin should be active for this monitor.

```rust
// In your plugin's start function
async fn start(
    &self,
    token: &CancellationToken,
    msg_logger: ArcMsgLogger,
    monitor: ArcMonitor,
) -> Result<(), StartError> {
    let config = monitor.config();

    // Your custom config parsing logic
    let Some(config) = YourObjectDetectionConfig::parse(config.raw().clone(), msg_logger.clone())?
    else {
        // Your plugin is not enabled for this monitor.
        return Ok(());
    };
    // ...
}
```

### 3.2. Selecting and Subscribing to the Video Stream

You can choose between the main stream and a lower-resolution sub-stream, which is often better for performance. The choice is typically a user-configurable option.

```rust
// In your plugin's start function

let source = if config.use_sub_stream {
    match monitor.source_sub().await {
        Some(Some(v)) => v,
        Some(None) => return Err(GetSubStream), // Sub-stream is enabled but not available
        None => {
            // Monitor was stopped before we could get the source
            return Ok(());
        }
    }
} else {
    match monitor.source_main().await {
        Some(v) => v,
        None => {
            // Monitor was stopped
            return Ok(());
        }
    }
};
```

Once you have the `source`, you subscribe to its decoded frames.

```rust
// In your plugin's run function, called from start

// You can use a FrameRateLimiter to control how many frames per second your plugin processes.
let rate_limiter =
    FrameRateLimiter::new(u64::try_from(*DurationH264::from(*config.feed_rate))?);

let Some(feed) = source
    .subscribe_decoded(
        self.rt_handle.clone(),
        msg_logger.clone(),
        Some(rate_limiter),
    )
    .await
else {
    // Cancelled
    return Ok(());
};
let mut feed = feed?;
```

### 3.3. The Frame Processing Loop

After subscribing, you'll enter a loop to receive and process frames.

```rust
// In your plugin's run function

loop {
    let Some(frame) = feed.recv().await else {
        // Feed was cancelled.
        return Ok(());
    };
    let frame = frame?;

    // 1. Pre-process the frame (e.g., convert color space, resize, crop).
    // This is often done in a blocking thread to avoid blocking the async runtime.
    let processed_frame_data = self.rt_handle.spawn_blocking(move || {
        process_frame(frame)
    }).await.expect("join")?;


    // 2. Send the processed frame to your detector.
    let Some(detections) = detector
        .detect(processed_frame_data)
        .await?
    else {
        // Detector was cancelled.
        return Ok(());
    };

    // 3. Post-process detections (e.g., apply thresholds and masks).
    let detections =
        parse_detections(&config.thresholds, &config.mask, &uncrop, detections)?;

    // 4. If objects are detected, trigger an event.
    if !detections.is_empty() {
        monitor
            .trigger(
                *config.trigger_duration,
                Event {
                    time: UnixNano::from(UnixH264::new(frame.pts())),
                    duration: *config.feed_rate,
                    detections,
                    source: Some("your_plugin_name".try_into().expect("valid")),
                },
            )
            .await?;
    }
}
```

## 4. Frame Pre-processing

Before sending a frame to the detector, it usually needs to be converted into a format the model understands (e.g., specific dimensions, color space, and data type). The `object_detection` plugin performs the following steps:

1.  **Grayscale**: Converts the YUV frame to grayscale by filling the U and V planes with `128`.
2.  **Scale**: Downscales the frame to a smaller resolution.
3.  **Convert**: Converts the pixel format to RGB24.
4.  **Pad**: Adds padding to match the aspect ratio of the detector's input.
5.  **Crop**: Crops the frame to the final dimensions required by the model.

Here is a simplified version of the `process_frame` function from `object_detection.rs`:

```rust
fn process_frame(s: &mut DetectorState, mut frame: Frame) -> Result<(), ProcessFrameError> {
    // Remove color
    let data = frame.data_mut();
    data[1].fill(128);
    data[2].fill(128);

    // Downscale
    let mut frame_scaled = Frame::new();
    let mut scaler = Scaler::new(
        frame.width(),
        frame.height(),
        frame.pix_fmt(),
        s.outputs.scaled_width,
        s.outputs.scaled_height,
    )?;
    scaler.scale(&frame, &mut frame_scaled)?;

    // Convert to rgb24
    let mut frame_converted = Frame::new();
    let mut converter = PixelFormatConverter::new(
        frame_scaled.width(),
        frame_scaled.height(),
        frame_scaled.color_range(),
        frame_scaled.pix_fmt(),
        PixelFormat::RGB24,
    )?;
    converter.convert(&frame_scaled, &mut frame_converted)?;

    // Add Padding
    let mut frame_padded = Frame::new();
    pad(
        &frame_converted,
        &mut frame_padded,
        s.outputs.padded_width,
        s.outputs.padded_height,
        0,
        0,
    )?;

    // Crop to final size
    let mut frame_cropped = Frame::new();
    crop(
        &frame_padded,
        &mut frame_cropped,
        s.outputs.crop_x,
        s.outputs.crop_y,
        s.outputs.output_width,
        s.outputs.output_height,
    )?;

    frame_cropped.copy_to_buffer(&mut s.frame_processed, 1)?;

    Ok(())
}
```

## 5. Conclusion

By following this design, a developer can create a new object detection plugin for SentryShot. The key is to correctly implement the `Plugin` trait, use the `on_monitor_start` hook to access the video feed, and then process the frames according to the specific requirements of the detection model. The existing `object_detection` plugin serves as a comprehensive example of how to handle configuration, frame processing, and event triggering.
