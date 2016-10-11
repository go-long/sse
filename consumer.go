package sse

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// A configConsumer represents a information about field retry in first event
// additional headers and w(ResponseWriter) for write these headers
type configConsumer struct {
	w      http.ResponseWriter
	retry  *time.Duration
	header map[string]string
}

// A Reconnect represents a information about recovery client, CID - id
// consumer`s, ID - id last event and StopRecovery. StopRecovery closes channel
// recovery and function MUST BE called in any case after send lost event to
// continue working
type Reconnect struct {
	CID      interface{}
	ID       string
	consumer *consumer
}

func (r *Reconnect) StopRecovery() {
	r.consumer.closeRecovery()
}

// A mxClose represents a mutex for waiting close event
type mxClose struct {
	sync.Mutex
	close bool
}

// A consumer represents a information about consumer, which main channel has to
// send events and reconnect channel to send priority events. Events is pushed into
// reconnect channel when consumer had made reconnect to server. Main channel
// not working when reconnect channel is working(pushing events) but safes sent event
// in the amount of 50 events
type consumer struct {
	mainChannel, recoveryChannel chan string
	context                      context.Context
	cancelContext                context.CancelFunc
	firstEvent                   struct {
		exec  bool
		retry time.Duration
	}
	waitCloseRecovery mxClose
	config            *configConsumer
}

// newConsumer creates new consumer and start waiting events
func newConsumer(cfg *configConsumer) *consumer {
	cons := &consumer{
		mainChannel:     make(chan string, 50),
		recoveryChannel: make(chan string, 50),
		config:          cfg,
	}
	// Set server side headers
	cons.config.w.Header().Set("Content-Type", "text/event-stream")
	cons.config.w.Header().Set("Cache-Control", "no-cache")
	cons.config.w.Header().Set("Connection", "keep-alive")
	// Set additional headers
	for hname, hvalue := range cons.config.header {
		cons.config.w.Header().Set(hname, hvalue)
	}

	return cons
}

// serve reads all event and sends it
func (c *consumer) serve(context context.Context, cancel context.CancelFunc) {
	c.context = context
	c.cancelContext = cancel
	go c.closeWait()
	// Cover panic if http was closed unexpectedly
	defer func() {
		recover()
	}()
	f, _ := c.config.w.(http.Flusher)
	// If reconnect happend, first reading will be priority events and just
	// after close recoveryChannel from main channel
	for message := range c.recoveryChannel {
		if !c.firstEvent.exec {
			c.addFieldRetry(&message)
		}
		fmt.Fprint(c.config.w, message)
		f.Flush()
	}
	for message := range c.mainChannel {
		if !c.firstEvent.exec {
			c.addFieldRetry(&message)
		}
		fmt.Fprint(c.config.w, message)
		f.Flush()
	}
}

// closeWait listens to the closing of the http connection via the CloseNotifier
// and context closing
func (c *consumer) closeWait() {
	// HTTP connection will be closed either consumer close itself or
	// its close server
	select {
	case <-c.config.w.(http.CloseNotifier).CloseNotify():
		c.cancelContext()
	case <-c.context.Done():
	}
}

// addFieldRetry adds by event retry field
func (c *consumer) addFieldRetry(message *string) {
	if !strings.Contains(*message, "retry") {
		tmp := *message
		*message = tmp[0:len(tmp)-2] +
			fmt.Sprintf("\nretry:%d\n\n", *c.config.retry/time.Millisecond)
	}
	c.firstEvent.exec = true
}

// recovery checks exist Last-Event-ID in http.Request
func (c *consumer) recovery(r *http.Request) (*Reconnect, bool) {
	if r.Header.Get("Last-Event-ID") != "" {
		return &Reconnect{
			CID:      r.Context().Value(consumerKey),
			ID:       r.Header.Get("Last-Event-ID"),
			consumer: c,
		}, true
	} else {
		c.closeRecovery()
		return nil, false
	}
}

// close disconnects consumer
func (c *consumer) close() {
	c.cancelContext()
}

// closeRecovery closes recovery channel to allow read from main channel
func (c *consumer) closeRecovery() {
	c.waitCloseRecovery.Lock()
	defer c.waitCloseRecovery.Unlock()
	if !c.waitCloseRecovery.close {
		close(c.recoveryChannel)
		c.waitCloseRecovery.close = true
	}
}
