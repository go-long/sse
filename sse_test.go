package sse

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func makeHandler(serveSSE SideEventer) func(w http.ResponseWriter, r *http.Request) {
	const COUNT = 7
	yield := make(chan int, COUNT)
	for i := 1; i <= COUNT; i++ {
		yield <- i
	}
	handler := func(w http.ResponseWriter, r *http.Request) {

		serveSSE.HandlerHTTP(<-yield, w, r)
	}
	return handler
}

func tinit(t *testing.T) (SideEventer, *httptest.Server, net.Conn) {
	serveSSE := New(&Config{
		Header: map[string]string{
			"Access-Control-Allow-Origin": "*",
		},
		Retry: time.Second * 15,
	})
	server := httptest.NewServer(http.HandlerFunc(makeHandler(serveSSE)))
	if conn, err := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1)); err != nil {
		t.Fatal(err)
	} else {
		time.Sleep(100 * time.Millisecond)
		conn.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
		return serveSSE, server, conn
	}
	return nil, nil, nil
}

var mx = sync.Mutex{}

func expectResponse(t *testing.T, c net.Conn, expecting string) {
	resp := read(t, c)
	if !strings.Contains(string(resp), expecting) {
		t.Errorf("expected:\n%s\ngot:\n%s\n", expecting, resp)
	}
}

func read(t *testing.T, c net.Conn) []byte {
	resp := make([]byte, 1024)
	_, err := c.Read(resp)
	if err != nil && err != io.EOF {
		t.Error(err)
	}
	return resp
}

func TestSendEventWithNotification(t *testing.T) {
	serveSSE, server, conn := tinit(t)
	defer server.Close()
	defer conn.Close()
	time.Sleep(100 * time.Millisecond)
	serveSSE.SendEvent(&Event{
		Event: "notification1",
		Data: &DataEvent{
			Value: "testMessage",
		},
	})
	expectResponse(t, conn, "event:notification1\ndata:testMessage\nretry:15000\n\n")
	serveSSE.Close()
}

func TestSendEventWithID(t *testing.T) {
	serveSSE, server, conn := tinit(t)
	defer server.Close()
	defer conn.Close()
	time.Sleep(100 * time.Millisecond)
	go func() {
		serveSSE.SendEvent(&Event{
			Data: &DataEvent{
				Value: "testMessage",
			},
			ID: "11",
		})
	}()
	expectResponse(t, conn, "data:testMessage\nid:11\nretry:15000\n\n")
	serveSSE.Close()
}

func TestSendEventFirstRetry(t *testing.T) {
	serveSSE, server, conn := tinit(t)
	defer server.Close()
	defer conn.Close()
	time.Sleep(100 * time.Millisecond)
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value: "testMessage",
		},
	})
	expectResponse(t, conn, "data:testMessage\nretry:15000\n\n")
	serveSSE.Close()
}

func TestRemoveNewline(t *testing.T) {
	serveSSE, server, conn := tinit(t)
	defer server.Close()
	defer conn.Close()
	time.Sleep(100 * time.Millisecond)
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value: "testMessage",
		},
		ID: "1\n1",
	})
	expectResponse(t, conn, "data:testMessage\nid:11\nretry:15000\n\n")
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value: "testMessage",
		},
		Event: "notification\n\n2",
	})
	expectResponse(t, conn, "event:notification2\ndata:testMessage\n\n")
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value: "testMessage",
		},
		Event: "notification\n2",
		ID:    "1\n\n\n\n1",
	})
	expectResponse(t, conn, "event:notification2\ndata:testMessage\nid:11\n\n")
	serveSSE.Close()
}

func TestCustomHeaderSSE(t *testing.T) {
	serveSSE, server, conn := tinit(t)
	defer server.Close()
	time.Sleep(100 * time.Millisecond)
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value: "testMessage",
		},
	})
	expectResponse(t, conn, "Access-Control-Allow-Origin: *\r\n")
	serveSSE.Close()
}

func TestManyEvents(t *testing.T) {
	serveSSE, server, conn := tinit(t)
	conn.Close()
	defer server.Close()
	conn1, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	conn2, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	conn1.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
	conn2.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
	time.Sleep(100 * time.Millisecond)
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value: "testMessage",
		},
	})
	expectResponse(t, conn1, "data:testMessage\nretry:15000\n\n")
	expectResponse(t, conn2, "data:testMessage\nretry:15000\n\n")
	conn1.Close()
	conn2.Close()
	conn, _ = net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	defer conn.Close()
	conn.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
	time.Sleep(time.Second)
	for count := 1; count <= 10; count++ {
		t.Logf("send message 'testMessage%d'", count)
		serveSSE.SendEvent(&Event{
			Data: &DataEvent{
				Value: "testMessage" + strconv.Itoa(count),
			},
		})
		expectResponse(t, conn, "data:testMessage"+strconv.Itoa(count)+"\n")
	}
	serveSSE.Close()
}

func TestConnectDisconnetClient(t *testing.T) {
	serveSSE := New(&Config{
		Retry: time.Second * 3,
	})
	var wg sync.WaitGroup
	var CID interface{}
	wg.Add(1)
	go func() {
		serveSSE.HandlerConnectNotify(func(id interface{}) {
			CID = id
		})
		serveSSE.HandlerDisconnectNotify(func(id interface{}) {
			if CID != id {
				t.Error("connect client`s uCID not equal client`s uCID")
			}
			wg.Done()
		})

	}()
	server := httptest.NewServer(http.HandlerFunc(makeHandler(serveSSE)))
	conn, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	conn.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
	time.Sleep(100 * time.Millisecond)
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value: "testMessage",
		},
	})
	conn.Close()
	wg.Wait()
	serveSSE.Close()
}

func TestConcurrentSendEvent(t *testing.T) {
	serveSSE, server, _ := tinit(t)
	defer server.Close()
	var wg sync.WaitGroup
	wg.Add(2)
	time.Sleep(time.Second)
	go func() {
		for count := 1; count <= 10; count++ {
			t.Logf("send message 'testMessage%d'", count)
			serveSSE.SendEvent(&Event{
				Data: &DataEvent{
					Value: "testMessage" + strconv.Itoa(count),
				},
			})
		}
		wg.Done()
	}()
	go func() {
		for count := 11; count <= 21; count++ {
			t.Logf("send message 'testMessage%d'", count)
			serveSSE.SendEvent(&Event{
				ID: "1",
				Data: &DataEvent{
					Value: "testMessage" + strconv.Itoa(count),
				},
			})
		}
		wg.Done()
	}()
	wg.Wait()
	serveSSE.Close()
}
func TestSendEventOnly(t *testing.T) {
	serveSSE := New(&Config{
		Retry: time.Second * 3,
	})
	channelCon := make(chan interface{}, 2)
	go func() {
		serveSSE.HandlerConnectNotify(func(id interface{}) {
			channelCon <- id
		})
	}()
	server := httptest.NewServer(http.HandlerFunc(makeHandler(serveSSE)))
	defer server.Close()
	conn1, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	time.Sleep(time.Second)
	conn2, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	conn1.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
	time.Sleep(time.Second)
	conn2.Write([]byte("GET / HTTP/1.1\nHost: fo1\n\n"))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		CID1 := <-channelCon
		for count := 1; count <= 10; count++ {
			time.Sleep(100 * time.Millisecond)
			t.Logf("send message 'testMessage%d'", CID1.(int))
			serveSSE.SendEvent(&EventOnly{
				CID: []interface{}{CID1},
				Data: &DataEvent{
					Value: "testMessage" + strconv.Itoa(CID1.(int)),
				},
			})
			expectResponse(t, conn1, "data:testMessage"+strconv.Itoa(CID1.(int))+"\n")
		}
		wg.Done()
	}()
	go func() {
		time.Sleep(100 * time.Microsecond)
		CID := <-channelCon
		for count := 1; count <= 10; count++ {
			time.Sleep(100 * time.Millisecond)
			t.Logf("send message 'testMessage%d'", CID.(int))
			serveSSE.SendEvent(&EventOnly{
				CID: []interface{}{CID},
				Data: &DataEvent{
					Value: "testMessage" + strconv.Itoa(CID.(int)),
				},
			})
			expectResponse(t, conn2, "data:testMessage"+strconv.Itoa(CID.(int))+"\n")
		}
		wg.Done()
	}()
	wg.Wait()
	serveSSE.Close()
}

func TestRecoveryFlushPanic(t *testing.T) {
	serveSSE, server, conn := tinit(t)
	defer serveSSE.Close()
	defer server.Close()
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value: "testMessage",
		},
	})
	conn.Close()
}

func TestRemoveConsumer(t *testing.T) {
	serveSSE := New(&Config{
		Retry: time.Second * 3,
	})
	var wg sync.WaitGroup
	var CID interface{}
	wg.Add(1)
	go func() {
		serveSSE.HandlerConnectNotify(func(id interface{}) {
			CID = id
		})
		serveSSE.HandlerDisconnectNotify(func(id interface{}) {
			if CID != id {
				t.Error("connect client`s uCID not equal client`s uCID")
			}
			wg.Done()
		})

	}()
	server := httptest.NewServer(http.HandlerFunc(makeHandler(serveSSE)))
	conn, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	conn.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
	time.Sleep(100 * time.Millisecond)
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value: "testMessage",
		},
	})
	serveSSE.RemoveConsumer(CID)
	wg.Wait()
	serveSSE.Close()
}

func TestCountConsumer(t *testing.T) {
	serveSSE := New(&Config{
		Retry: time.Second * 3,
	})
	server := httptest.NewServer(http.HandlerFunc(makeHandler(serveSSE)))
	conn1, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	conn1.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
	conn2, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	conn2.Write([]byte("GET / HTTP/1.1\nHost: fo1\n\n"))
	time.Sleep(100 * time.Millisecond)
	if serveSSE.CountConsumer() != 2 {
		t.Errorf("expect: 2\ngot: %d", serveSSE.CountConsumer())
	}
	conn2.Close()
	time.Sleep(100 * time.Millisecond)
	if serveSSE.CountConsumer() != 1 {
		t.Errorf("expect: 1\ngot: %d", serveSSE.CountConsumer())
	}
	conn1.Close()
	time.Sleep(100 * time.Millisecond)
	if serveSSE.CountConsumer() != 0 {
		t.Errorf("expect: 0\ngot: %d", serveSSE.CountConsumer())
	}
	serveSSE.Close()
}

func TestSendEventExcept(t *testing.T) {
	serveSSE := New(&Config{
		Retry: time.Second * 3,
	})
	channelCon := make(chan interface{}, 2)
	go func() {
		serveSSE.HandlerConnectNotify(func(id interface{}) {
			channelCon <- id
		})
	}()
	server := httptest.NewServer(http.HandlerFunc(makeHandler(serveSSE)))
	defer server.Close()
	conn1, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	time.Sleep(time.Second)
	conn2, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	conn1.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
	time.Sleep(100 * time.Millisecond)
	conn2.Write([]byte("GET / HTTP/1.1\nHost: fo1\n\n"))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		CID := <-channelCon
		for count := 1; count <= 10; count++ {
			time.Sleep(100 * time.Millisecond)
			t.Logf("send message 'testMessage%d'", CID.(int))
			serveSSE.SendEvent(&EventExcept{
				CID: []interface{}{CID},
				Data: &DataEvent{
					Value: "testMessage" + strconv.Itoa(CID.(int)),
				},
			})
			expectResponse(t, conn2, "data:testMessage"+strconv.Itoa(CID.(int))+"\n")
		}
		wg.Done()
	}()
	go func() {
		time.Sleep(100 * time.Microsecond)
		CID := <-channelCon
		for count := 1; count <= 10; count++ {
			time.Sleep(100 * time.Millisecond)
			t.Logf("send message 'testMessage%d'", CID.(int))
			serveSSE.SendEvent(&EventExcept{
				CID: []interface{}{CID},
				Data: &DataEvent{
					Value: "testMessage" + strconv.Itoa(CID.(int)),
				},
			})
			expectResponse(t, conn1, "data:testMessage"+strconv.Itoa(CID.(int))+"\n")
		}
		wg.Done()
	}()
	wg.Wait()
	serveSSE.Close()
}

func TestSendEventNewLine(t *testing.T) {
	serveSSE, server, conn := tinit(t)
	defer server.Close()
	defer conn.Close()
	time.Sleep(100 * time.Millisecond)
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value: "testMessage1\ntestMessage2",
		},
	})
	expectResponse(t, conn, "data:testMessage1\ndata:testMessage2\nretry:15000\n\n")
	serveSSE.Close()
}

func TestSendEventDisabledFormatting(t *testing.T) {
	serveSSE, server, conn := tinit(t)
	defer server.Close()
	defer conn.Close()
	time.Sleep(100 * time.Millisecond)
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value:              "testMessage1\ntestMessage2",
			DisabledFormatting: true,
		},
	})
	expectResponse(t, conn, "data:testMessage1\ntestMessage2\nretry:15000\n\n")
	serveSSE.Close()
}

func TestSendEventRetry(t *testing.T) {
	serveSSE, server, conn := tinit(t)
	defer server.Close()
	time.Sleep(100 * time.Millisecond)
	serveSSE.SendEvent(&EventRetry{
		Time: time.Second * 4,
	})
	expectResponse(t, conn, "retry:4000\n\n")
	conn.Close()
	conn1, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	conn1.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
	time.Sleep(100 * time.Millisecond)
	serveSSE.SendEvent(&Event{
		Data: &DataEvent{
			Value: "testMessage",
		},
	})
	expectResponse(t, conn1, "data:testMessage\nretry:4000\n\n")
	serveSSE.Close()
}

func TestIdenticalCID(t *testing.T) {
	serveSSE := New(&Config{
		Retry: time.Second * 3,
	})
	label := false
	serveSSE.HandlerDisconnectNotify(func(id interface{}) {
		label = true
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveSSE.HandlerHTTP(1, w, r)
	}))
	defer server.Close()
	conn, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	time.Sleep(100 * time.Millisecond)
	conn.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
	time.Sleep(100 * time.Millisecond)
	conn1, _ := net.Dial("tcp", strings.Replace(server.URL, "http://", "", 1))
	time.Sleep(100 * time.Millisecond)
	conn1.Write([]byte("GET / HTTP/1.1\nHost: foo\n\n"))
	conn1.Close()
	if label {
		t.Fatal("identical CID")
	}
	serveSSE.Close()
}
