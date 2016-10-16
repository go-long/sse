Simple golang server side events for net/http server, go 1.7
==
#### [Example](https://github.com/itcomusic/sse/tree/master/example)
## Start using it

1. Download and install it:

```sh
go get github.com/itcomusic/sse
```

2. Import it in your code:

```go
import "github.com/itcomusic/sse"
```

## Create sse

#### SSE with default options
Field ```Retry``` is seconds waitting reconnect client to server after
connection had lost.

```go
import "github.com/itcomusic/sse"
handleSSE := sse.New(&sse.Config{
        Retry: time.Second * 3,
})
```

#### SSE with custom HTTP headers

```go
import "github.com/itcomusic/sse"
handleSSE := sse.New(&sse.Config{
    Retry:                   time.Second * 3,
    Header: map[string]string{
        "Access-Control-Allow-Origin": "*",
    },
})
```

#### Handler  
First argument function HandlerHTTP must be cid client's. CID must be unique.
CID is key for save clients(consumers). CID can be generate with uuid packages or
smt else. Identical CID is prohibited, if CID are found, new client do not
connect to server.

```go
import "github.com/itcomusic/sse"
handleSSE := sse.New(&sse.Config{
    Retry: time.Second * 3,
})

http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
    handleSSE.HandlerHTTP("cid", w, r)
})
```

## API Example
Every event with ```Data``` has optional attribute ```DisabledFormatting```. If
it is enabled, ```Data``` will be formatted.
You can disabled formatting. It is necessary to deny creating additional
fields ```data```. If you want send json data and it contains ```\n```, desirable
set ```DisabledFormatting = true```.


#### Send Event
Send event all clients

```go
import "github.com/itcomusic/sse"
handleSSE := sse.New(&sse.Config{
    Retry: time.Second * 3,
})
serveSSE.SendEvent(&Event{
	Data: &DataEvent{
		Value: "testMessage",
	},
})
```

#### Send EventOnly
Send event only to selected clients. CID are slice IDs

```go
import "github.com/itcomusic/sse"
handleSSE := sse.New(&sse.Config{
    Retry: time.Second * 3,
})
serveSSE.SendEvent(&EventOnly{
    CID: []interface{}{"cid1", "cid2"},
    Data: &DataEvent{
        Value: "testMessageOnly",
    },
})
```

#### Send EventExcept
Send event except to selected clients. CID are slice IDs

```go
import "github.com/itcomusic/sse"
handleSSE := sse.New(&sse.Config{
    Retry: time.Second * 3,
})
serveSSE.SendEvent(&EventExcept{
    CID: []interface{}{"cid1", "cid2"},
    Data: &DataEvent{
        Value: "testMessageOnly",
    },
})
```

#### Send EventRetry
Send event which it has time in seconds waitting reconnect client to server after
connection had lost. Also new clients will be get this time.

```go
import "github.com/itcomusic/sse"
handleSSE := sse.New(&sse.Config{
    Retry: time.Second * 3,
})
serveSSE.SendEvent(&EventRetry{
    Time: time.Second * 15,
})
```

#### Count connections
Gives information about count connected clients, including inactive -
client has not been removed yet from map.

```go
import "github.com/itcomusic/sse"
handleSSE := sse.New(&sse.Config{
    Retry: time.Second * 3,
})
serveSSE.CountConsumer()
```

## Notify
Notifications inform about connected, disconnected, reconnected clients

#### ConnectNotify

```go
import "github.com/itcomusic/sse"
handleSSE := sse.New(&sse.Config{
    Retry: time.Second * 3,
})
serveSSE.HandlerConnectNotify(func(cid interface{}) {
    // cid is id (client)consumer, when connected and ready
})
```

#### DisconnectNotify

```go
import "github.com/itcomusic/sse"
handleSSE := sse.New(&sse.Config{
    Retry: time.Second * 3,
})
serveSSE.HandlerDisconnectNotify(func(cid interface{}) {
    // cid is id (client)consumer, when he disconnected from server side event
})
```

#### ReconnectNotify
Reconnect notify execute when client made reconnect to server side event.
Server can send events and client do not get them, because connection with server
was lost. ReconnectNotify will decide this problem. It allows
detect ID event, and you can send lost events. After sending lost events, must
close recovery channel, call ```StopRecovery()```

```go
import "github.com/itcomusic/sse"
handleSSE := sse.New(&sse.Config{
    Retry: time.Second * 3,
})
serveSSE.HandlerReconnectNotify(func(rec *sse.Reconnect interface{}) {
    // Reconnect struct shows information about client, which reconnected
    // to server side event.
    defer rec.StopRecovery()
})
```
