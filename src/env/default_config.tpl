# Port app will be served on.
port = 2020

# Directory where recordings will be stored.
storage_dir = "{{ cwd }}/storage"

# Directory where configs will be stored.
config_dir = "{{ cwd }}/configs"

# Directory where the plugins are located.
plugin_dir = "{{ cwd }}/plugins"


# Maximum allowed storage space in GigaBytes.
# Recordings are delete automatically before this limit is exceeded.
max_disk_usage = 100

# Write debug logs to stdout. Still available from the logs page.
debug_log_stdout = false


# PLUGINS

# Authentication. One must be enabled.

# Basic Auth.
[[plugin]]
name = "auth_basic"
enable = false

# No authentication.
[[plugin]]
name = "auth_none"
enable = false



# Motion detection.
# Unreliable in most situations.
[[plugin]]
name = "motion"
enable = false

# Object detection.
# Enabling will generate a `object_detction.toml` file.
[[plugin]]
name = "object_detection"
enable = false


# Thumbnail downscaling.
# Downscale video thumbnails to improve page load times and data usage.
[[plugin]]
name = "thumb_scale"
enable = false


# MQTT API.
# Documentation: ./docs/4_API.md
[[plugin]]
name = "mqtt"
enable = false
host = "127.0.0.1"
port = 1883
