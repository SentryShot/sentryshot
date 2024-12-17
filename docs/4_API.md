#### Warning: undocumented APIs do not have any stability guarantees and may change without warning.

-   [MQTT API](#mqtt-api)
-   [REST API](#rest-api)
    -   [Account](#Account)
    -   [Monitor](#monitor)
    -   [Logs](#logs)
-   [Websockets API](#websockets-api)
    -   [Logs](#logs)


<br>

# MQTT API

Enable the `mqtt` plugin and configure the external broker address in `sentryshot.toml`

### sentryshot/events/tflite

``` json
{
  "monitorID": "one",
  "monitorName": "camera_1",
  "label": "person",
  "score": 63.671875,
  "time": "2024-11-20T14:23:15.437494909Z",
  "source": "tflite"
}
```

### sentryshot/events/motion

``` json
{
  "monitorID": "one",
  "monitorName": "camera_1",
  "label": "zone0",
  "score": 48.39815,
  "time": "2024-11-20T14:23:15.437494909Z",
  "source": "motion"
}
```

<br>
<br>

# REST API

There is a `/api` page where you can try the endpoints.

All requests require basic auth, POST, PUT and DELETE requests need to have a matching CSRF-token in the `X-CSRF-TOKEN` header.

##### curl examples:

``` shell
curl -u user:pass -X GET https://127.0.0.1/api/accounts

TOKEN=$(curl -k -u user:pass -X GET https://127.0.0.1:2020/api/account/my-token)
#printf "token: %s\n" "$TOKEN"
curl -k -u user:pass -X DELETE https://127.0.0.1:2020/api/account?id=x -H "X-CSRF-TOKEN: $TOKEN"
```
<br>

## Account

### DELETE /api/account?id=x

##### Auth: admin

Delete account.

<br>

### PUT /api/account

Create or replace account.

##### Auth: admin

example request:

```
{
	"id": "7phg3h7v3ayb5g2f",
	"username": "name",
	"isAdmin": false,
	"plainPassword": "pass"
}
```

<br>

### GET /api/account/my-token

##### Auth: user

Returns the temporary CSRF-token for your account.

<br>

### GET /api/accounts

##### Auth: admin

JSON response of all accounts.

<br>
<br>

## Monitor

### DELETE /api/monitor?id=x

##### Auth: admin

Delete monitor.

<br>

### PUT /api/monitor

##### Auth: admin

Set monitor json config.

<br>

### GET /api/monitors

##### Auth: admin

All monitor json configs.

<br>

### PATCH /api/monitor/<MONITOR_ID>/motion/enable
### PATCH /api/monitor/<MONITOR_ID>/motion/disable
### PATCH /api/monitor/<MONITOR_ID>/tflite/enable
### PATCH /api/monitor/<MONITOR_ID>/tflite/disable


##### Auth: admin

Toggle detector and restart monitor.

<br>
<br>

## Logs

### GET /api/log/query?levels=error,warning&sources=app,monitors=a,b&time=1234567890111222&limit=2

##### Auth: admin

Query logs. Time is in Unix micro seconds.

example response:

```
[
  {
    "level": "warning",
    "time": 0,
    "msg": "",
    "src": "",
    "monitorID": ""
  },
  {
    "level": "warning",
    "time": 0,
    "msg": "",
    "src": "",
    "monitorID": ""
  }
]
```


<br>
<br>

# Websockets API

Requires TLS. Authentication is validated before each response.

example: `wss://127.0.0.1/api/logs`

curl doesn't support wss.

## Logs

### /api/logs?levels=error,warning&monitors=a,b&sources=app,monitor

##### Auth: admin

Live log feed.
