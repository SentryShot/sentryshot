-   [REST API](#rest-api)
    -   [Monitor](#monitor)
    -   [Account](#Account)
    -   [Logs](#logs)
-   [Websockets API](#websockets-api)
    -   [Logs](#logs)

## Monitor

### DELETE /api/monitor

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
<br>

## Account

### GET /api/accounts

##### Auth: admin

Users.

<br>

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
