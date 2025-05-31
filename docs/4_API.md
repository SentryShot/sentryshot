#### Warning: undocumented APIs do not have any stability guarantees and may change without warning.

-   [MQTT API](#mqtt-api)
-   [HTTP API](#http-api)
    -   [Account](#Account)
    -   [Monitor](#monitor)

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

# HTTP API

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

### GET /api/account/my-token

##### Auth: user

Returns the temporary CSRF-token for your account.

<br>
<br>

## Monitor


### PATCH /api/monitor/<MONITOR_ID>/motion/enable
### PATCH /api/monitor/<MONITOR_ID>/motion/disable
### PATCH /api/monitor/<MONITOR_ID>/object-detection/enable
### PATCH /api/monitor/<MONITOR_ID>/object-detection/disable


##### Auth: admin

Toggle detector and restart monitor.


