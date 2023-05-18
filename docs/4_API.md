-   [Re-streaming](#re-streaming)
-   [REST API](#rest-api)
    -   [System](#system)
    -   [General](#general)
    -   [User](#user)
    -   [Monitor](#monitor)
    -   [Recording](#recording)
    -   [Logs](#logs)
-   [Websockets API](#websockets-api)
    -   [Logs](#logs)

# Re-streaming

## RTSP

### Main rtsp\://127.0.0.1:2021/\<monitor-id\>

### Sub rtsp\://127.0.0.1:2021/\<monitor-id\>\_sub

##### example:

    ffplay -rtsp_transport tcp rtsp://127.0.0.1:2021/myMonitor
    ffplay -rtsp_transport tcp rtsp://127.0.0.1:2021/myMonitor_sub

Remember to expose the ports if you're using Docker.

## HLS

### Main http\://127.0.0.1:2022/hls/<monitor-id\>/stream.m3u8

### Sub http\://127.0.0.1:2022/hls/<monitor-id\>\_sub/stream.m3u8

##### example:

    ffplay http://127.0.0.1:2022/hls/myMonitor/stream.m3u8
    vlc http://127.0.0.1:2022/hls/myMonitor_sub/stream.m3u8

<br>
<br>

# REST API

All requests require basic auth, POST, PUT and DELETE requests need to have a matching CSRF-token in the `X-CSRF-TOKEN` header.

##### curl examples:

    curl -k -u admin:pass -X GET https://127.0.0.1/api/users

    TOKEN=$(curl -k -u admin:pass -X GET https://127.0.0.1/api/user/my-token)
    printf "token: %s\n" "$TOKEN"
    curl -k -u admin:pass -X POST https://127.0.0.1/api/monitor/restart?id=x -H "X-CSRF-TOKEN: $TOKEN"


## System

### GET /api/system/time-zone

##### Auth: user

System time zone location.

<br>

## General

### GET /api/general

##### Auth: admin

General settings.

Example response:`{"diskSpace":"20","theme":"default"}`

<br>

### PUT /api/general/set

##### Auth: admin

Set general configuration.

Example request:`{"diskSpace":"21","theme":"default"}`

<br>

## User

### GET /api/users

##### Auth: admin

Users.

Example response:

```
{
  "11":{
    "id":"11",
    "username":"admin",
    "isAdmin":true
  },
  "22":{
    "id":"22",
    "username":
    "user","isAdmin":false
  }
}
```

Note: the name of the object matches the account ID.

<br>

### PUT /api/user/set

Set user data.

##### Auth: admin

Example request:

```
{
	"id": "7phg3h7v3ayb5g2f",
	"username": "name",
	"isAdmin": false,
	"plainPassword": "pass"
}
```



<br>

### DELETE /api/user/delete?id=x

##### Auth: admin

Delete a user by id.

<br>

### GET /api/user/my-token

##### Auth: admin

CSRF-token of current user.

<br>

## Monitor

### GET /api/monitor/configs

##### Auth: admin

Uncensored monitor configuration.

Example response:

```
{
  "111": {
    "id": "111",
    "name": "a",
    "enable": "true"
    // More fields.
  },
  "222": {
    "id": "222",
    "name": "b",
    "enable": "false"
    // More fields.
  }
}
```

Note: the name of the object matches the monitor ID.

<br>

### DELETE /api/monitor/delete?id=x

##### Auth: admin

Delete a monitor by id.

<br>

### GET /api/monitor/list

##### Auth: user

Censored monitor configuration.

```
{
  "111": {
    "audioEnabled":"false",
    "enable":"true",
    "id":"111",
    "name":"a",
    "subInputEnabled":"false"
  },
  "222":{
    "audioEnabled":"false",
    "enable":"false",
    "id":"222",
    "name":"b",
    "subInputEnabled":"false"
  }
}
```

<br>

### POST /api/monitor/restart?id=x

##### Auth: admin

Restart monitor by id.

<br>

### PUT /api/monitor/set

##### Auth: admin

Create/update monitor configuration.

Example request:

```
{
  "id": "111",
  "name": "a",
  "enable": "true",
  "inputOptions": "x",
  "mainInput": "x",
  "subInput": "x",
  "hwaccel": "hwaccel",
  "videoEncoder": "copy",
  "audioEncoder": "none",
  "alwaysRecord": "false",
  "videoLength": "15",
  "timestampOffset": "500",
  "logLevel": "fatal"
}
```

The `id` field is used to determine the monitor to create/update.

There is currently no way to get the config for a single monitor, `/api/monitor/configs` can be used to get all of them at once.

Use the `/api/monitor/restart?id=x` endpoint to restart the monitor and make the changes take effect.

<br>

## Recording

The recording ID is a string in the following format and has multiple matching files with the same name in the recordings directory. All timestamps in the back-end use the UTC timezone.

Format:`YYYY-MM-DD_hh-mm-ss_MonitorID`

Example `2020-12-31_23-59-59_x`

See [crawler.go](../pkg/storage/crawler.go) for more info.

<br>

### DELETE /api/recording/delete/\<recording-id>

##### Auth: admin

Delete recording by id.

<br>

### GET /api/recording/thumbnail/\<recording-id>

##### Auth: user

Thumbnail by exact recording ID.

<br>

### GET /api/recording/video/\<recording-id>

##### Auth: user

Video by exact recording ID.

curl example:

    curl -k -u admin:pass -X GET https://127.0.0.1/api/recording/video/2025-12-28_23-59-59

<br>

### GET /api/recording/query?limit=1&time=2025-12-28_23-59-59&reverse=true&monitors=m1,m2&data=true

##### Auth: user

Query recordings. The time parameter can accept a recording ID and will check if a recording with that exact id exist on disk and if true will start returning subsequent alphabetically ordered recordings but not the recording itself. If an exact match isn't found, it will start from the closest match.

See the test cases in [crawler_test.go](../pkg/storage/crawler_test.go)

Example request:

    /api/recording/query?limit=1&time=9999-12-28_23-59-59&data=true

Example response: data=false

```
[
  {
    "id":"YYYY-MM-DD_hh-mm-ss_id",
    "data": null
  }
]
```

Example response: data=true

```
[{
  "id":"YYYY-MM-DD_hh-mm-ss_id",
  "data": {
    "start": "YYYY-MM-DDThh:mm:ss.000000000Z",
    "end": "YYYY-MM-DDThh:mm:ss.000000000Z",
    "events": [{
        "time": "YYYY-MM-DDThh:mm:ss.000000000Z",
        "detections": [{
            "label": "person",
            "score": 100,
            "region": {
              "rect": [0, 0, 100, 100]
            }
        }],
        "duration": 000000000
}]}}]
```

<br>
## Logs

### GET /api/log/query?levels=16,24&sources=app,monitors=a,b&time=1234567890111222&limit=2

##### Auth: admin

Query logs. Time is in Unix micro seconds.

Example response:

```
[
  {
    "level":0,
    "time":0,
    "msg":"",
    "src":"",
    "monitorID":""
  },
  {
    "level":0,
    "time":0,
    "msg":"",
    "src":"",
    "monitorID":""
  }
]
```

<br>

### GET /api/log/sources

##### Auth: admin

List of log sources.

Example response:`["app","monitor","recorder","storage","watchdog"]`

<br>
<br>

# Websockets API

Requires basic auth and TLS. Authentication is validated before each response.

Example: `wss://127.0.0.1/api/logs`

curl doesn't support wss.

## Logs

### /api/logs?levels=16,24&monitors=a,b&sources=app,monitor

##### Auth: admin

Live log feed.
