The video server is a minimal version of rtsp-simple-server and gortsplib.
Resulting binary was 12MB smaller.

```
                  Input
                    |
                    v
                  FFmpeg
                    |
                    v
            +--Video-Server--+
            |                |
            v                v
Browser <--HLS              RTSP--> Other
            |                |
            v                v
         Recorder     Object-Detection
```

The input is first passed through FFmpeg where it's converted to a supported format for the video server and optionally transcoded.

The video server supports 2 protocols.

HLS caches a few seconds of video that is used by the recorder to start the recording a few seconds before it's triggered.

RTSP is used by internal components like object-detection to access a instant feed of the camera.
