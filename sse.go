// Package sse is simple golang server side events
package sse

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type keyContext int

const (
	consumerKey keyContext = iota
	consumerValue
)

// A Config represents a config to run SSE
type Config struct {
	Header map[string]string
	Retry  time.Duration
}

type mpConsumer struct {
	sync.RWMutex
	value map[interface{}]*consumer
}

// A SideEventer represents a interface SSE
type SideEventer interface {
	SendEvent(eventer)
	HandlerConnectNotify(func(interface{}))
	HandlerDisconnectNotify(func(interface{}))
	HandlerReconnectNotify(func(*Reconnect))
	RemoveConsumer(interface{})
	CountConsumer() int
	HandlerHTTP(interface{}, http.ResponseWriter, *http.Request)
	Close()
}

// A SSE represents a information about consumers. SSE has a map of consumers,
// the keys of the map are the СID which we can push events to attached clients
type SSE struct {
	consumer *mpConsumer
	closeSSE chan bool
	event    chan eventer
	// Informations about situations (optional)
	handlerConnectNotify    func(interface{})
	handlerDisconnectNotify func(interface{})
	handlerReconnectNotify  func(*Reconnect)
	waitClose               struct {
		sync.Mutex
		sync.WaitGroup
		denyConnections  bool
		countConnections int
	}
	config Config
}

// New creates server side event and starts wait get events
func New(cfg *Config) SideEventer {
	sse := &SSE{
		consumer: &mpConsumer{
			value: make(map[interface{}]*consumer),
		},
		closeSSE: make(chan bool, 1),
		event:    make(chan eventer, 50),
		config:   *cfg,
	}

	sse.start()
	return sse
}

// receiveEvent waits new events and dispatches them
func (s *SSE) receiveEvent() {
	for event := range s.event {
		event.dispatch(s.consumer)
		if eventRetry, ok := event.(*EventRetry); ok {
			s.config.Retry = eventRetry.Time
		}
	}
}

// closeWait closes all channel and disconnects all clients
func (s *SSE) closeWait() {
	select {
	case <-s.closeSSE:
		close(s.closeSSE)
		close(s.event)
		s.waitClose.Lock()
		s.waitClose.denyConnections = true
		s.waitClose.Unlock()
		s.consumer.RLock()
		for _, cons := range s.consumer.value {
			cons.close()
		}
		s.consumer.RUnlock()
	}
}

func (s *SSE) start() {
	go s.receiveEvent()
	go s.closeWait()
}

// SendEvent sends event
func (s *SSE) SendEvent(event eventer) {
	s.event <- event
}

// RemoveConsumer removes consumer by СID
func (s *SSE) RemoveConsumer(сid interface{}) {
	s.consumer.RLock()
	if cons, ok := s.consumer.value[сid]; ok {
		cons.close()
	}
	s.consumer.RUnlock()
}

// HandlerConnectNotify calls function
func (s *SSE) HandlerConnectNotify(handler func(interface{})) {
	s.handlerConnectNotify = handler
}

// HandlerReconnectNotify calls function and transfers struct Reconnect.
// It uses if need recovery lost event consumer`s
func (s *SSE) HandlerReconnectNotify(handler func(*Reconnect)) {
	s.handlerReconnectNotify = handler
}

// HandlerDisconnectNotify calls function
func (s *SSE) HandlerDisconnectNotify(handler func(interface{})) {
	s.handlerDisconnectNotify = handler
}

// add adds new client in map
// unlocks map
func (s *SSE) add(ctx context.Context) {
	s.consumer.value[ctx.Value(consumerKey)] = ctx.Value(consumerValue).(*consumer)
	s.consumer.Unlock()
	if s.handlerConnectNotify != nil {
		s.handlerConnectNotify(ctx.Value(consumerKey))
	}
}

// CountConsumer returns count clients, include active and noactive.
// No consistency, because many events such as disconnect, connect or remove
// consumers have not been executed yet
func (s *SSE) CountConsumer() int {
	s.consumer.RLock()
	defer s.consumer.RUnlock()
	return len(s.consumer.value)
}

// HandlerHTTP handles new connections
// Creates new context information about client.
func (s *SSE) HandlerHTTP(cid interface{}, w http.ResponseWriter, r *http.Request) {
	// Check wait close side event
	s.waitClose.Lock()
	// Make sure that the writer support flushing
	if _, ok := w.(http.Flusher); !ok || s.waitClose.denyConnections {
		s.waitClose.Unlock()
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.waitClose.Unlock()
	s.consumer.Lock()
	if _, ok := s.consumer.value[cid]; ok {
		s.consumer.Unlock()
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), consumerKey, cid))
	consumer := newConsumer(&configConsumer{
		w:      w,
		retry:  &s.config.Retry,
		header: s.config.Header,
	})
	ctx = context.WithValue(ctx, consumerValue, consumer)
	go consumer.serve(ctx, cancel)
	s.add(ctx)
	// Check recconnect consumer
	// Create new context with id consumer
	r = r.WithContext(context.WithValue(r.Context(), consumerKey, cid))
	if info, ok := consumer.recovery(r); ok {
		if s.handlerReconnectNotify != nil {
			s.handlerReconnectNotify(info)
		} else {
			consumer.closeRecovery()
		}
	}
	// Waits when context will be cancel
	<-ctx.Done()
	// Remove consumer from map
	s.consumer.Lock()
	if consumer, ok := s.consumer.value[ctx.Value(consumerKey)]; ok {
		close(consumer.mainChannel)
		delete(s.consumer.value, ctx.Value(consumerKey))
	}
	s.consumer.Unlock()
	// Sends notification about disconnected
	if s.handlerDisconnectNotify != nil {
		s.handlerDisconnectNotify(ctx.Value(consumerKey))
	}
	/*
		Don't close the connection, instead loop 10 times,
		sending messages and flushing the response each time
		there is a new message to send along.

		NOTE: we could loop endlessly; however, then you
		could not easily detect clients that detach and the
		server would continue to send them messages long after
		they're gone due to the "keep-alive" header.  One of
		the nifty aspects of SSE is that clients automatically
		reconnect when they lose their connection.
	*/
}

// Close closes side event: close all connections, close all channel
func (s *SSE) Close() {
	s.closeSSE <- true
}
