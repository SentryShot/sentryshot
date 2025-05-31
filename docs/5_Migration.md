# v0.2.0 -> v0.3.0

## "tflite" plugin renamed to "object_detection"

Update the plugin name in `sentryshot.toml`

``` diff
 [[plugin]]
-name = "tflite"
+name = "object_detection"
 enable = false
 ```

No further action is required if you've never enabled the tflite plugin. If you have enabled it but don't care about the config, simply delete `tflite.toml`

Backup `tflite.toml` and rename it to `object_detection.toml`

``` shell
cd configs
cp tflite.toml tflite.toml.backup
mv tflite.toml object_detection.toml
```

Rename `detector_cpu` to `detector_tflite` in `object_detection.toml`

``` diff
-[[detector_cpu]]
+[[detector_tflite]]
```