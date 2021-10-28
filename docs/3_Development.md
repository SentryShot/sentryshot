-   [REST API](#rest-api)
	-   [System](#system)
	-   [General](#general)
	-   [User](#user)
	-   [Monitor](#monitor)
	-   [Recording](#recording)
	-   [Logs](#logs)
-   [Websockets API](#websockets-api)
	-   [Logs](#logs)

# REST API

All requests require basic auth, POST, PUT and DELETE requests need to have a matching CSRF-token in the `X-CSRF-TOKEN` header.

##### curl example:

    curl -k -u admin:pass -X GET https://127.0.0.1/api/users

## System

### GET /api/system/timeZone

##### Auth: user

System time zone location.

<br>

## General

### GET /api/general

##### Auth: admin

General settings.

<br>

### PUT /api/general/set

##### Auth: admin

Set general configuration.

<br>

## User

### GET /api/users

##### Auth: admin

Users.

<br>

### PUT /api/user/set

##### Auth: admin

Set user data.

<br>

### DELETE /api/user/delete?id=x

##### Auth: admin

Delete a user by id.

<br>

### GET /api/user/myToken

##### Auth: admin

CSRF-token of current user.

<br>

## Monitor

### GET /api/monitor/list

##### Auth: user

Censored monitor configuration.

<br>

### GET /api/monitor/configs

##### Auth: admin

Uncensored monitor configuration.

<br>

### POST /api/monitor/restart?id=x

##### Auth: admin

Restart monitor by id.

<br>

### SET /api/monitor/set

##### Auth: admin

Set monitor.

<br>

### DELETE /api/monitor/delete?id=x

##### Auth: admin

Delete a monitor by id.

<br>

## Recording

### GET /api/recording/query?limit=2&before=2025-12-28_23-59-59&reverse=true

##### Auth: user

Query recordings.

example response:

```
[
  {
    "id":"YYYY-MM-DD_hh-mm-ss_id",
    "path":"storage/recordings/YYYY/MM/DD/id/YYYY-MM-DD_hh-mm-ss_id"
  },
  {
    "id":"YYYY-MM-DD_hh-mm-ss_id",
    "path":"storage/recordings/YYYY/MM/DD/id/YYYY-MM-DD_hh-mm-ss_id"
  }
]
```

<br>
## Logs

### GET /api/log/query?levels=16,24&sources=app,monitor&time=10000000000000000&limit=2

##### Auth: admin

Query logs. Time is in Unix milliseconds.

example response:

```
[
  {
    "Level":0,
    "Time":0,
    "Msg":"",
    "Src":"",
    "Monitor":""
  },
  {
    "Level":0,
    "Time":0,
    "Msg":"",
    "Src":"",
    "Monitor":""
  }
]
```

<br>

### GET /api/log/sources

##### Auth: admin

List of log sources.

example response:`["app","monitor","recorder","storage","watchdog"]`


<br>
<br>

# Websockets API

Requires basic auth and TLS. Authentication is validated before each response.

example: `wss://127.0.0.1/api/logs`

curl doesn't support wss.

## Logs

### /api/logs?levels=16,24&sources=app,monitor

##### Auth: admin

Live log feed.
