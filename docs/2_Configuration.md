# Configuration

- [Monitors](#monitors)
	- [ID](#id)
	- [Name](#name)
	- [Enable monitor](#enable-monitor)
	- [Source rtsp](#source-rtsp)
	- [Always record](#always-record)
	- [Video length](#video-length)

- [Accounts](#accounts)

<br>

## Monitors

### ID
Monitor identifier. The monitors recordings are tied to this id.

### Name
Arbitrary display name, can probably be any ASCII character.

### Enable monitor
Enable or Disable the monitor.

### Source rtsp

```
# Protocol
TCP or UDP

# Main input
Main camera feed, full resolution. Used when recording.

# Sub input
If your camera support a sub stream of lower resolution. Both inputs can be viewed from the live page.
```

### Always record
Always record.

### Video Length
Maximum video length in minutes.

<br>

## Accounts
##### Fields: 

```
# Username
Name of user.

# Admin
If user has admin privileges or not.

# New password
Set initial or change password.

# Repeat password
Confirm password.
```