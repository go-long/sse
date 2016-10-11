package sse

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// A DataEvent presents a main field in every event. Contains a data value
type DataEvent struct {
	Value              string
	DisabledFormatting bool
}

// formattingEvent creates format server side event
func formattingEvent(event string, data struct {
	Value              string
	DisabledFormatting bool
}, id string) string {
	var eventMsg bytes.Buffer
	if event != "" {
		eventMsg.WriteString(fmt.Sprintf("event:%s\n", strings.Replace(event, "\n", "", -1)))
	}
	if data.Value != "" {
		if !data.DisabledFormatting {
			lines := strings.Split(data.Value, "\n")
			for _, line := range lines {
				eventMsg.WriteString(fmt.Sprintf("data:%s\n", line))
			}
		} else {
			eventMsg.WriteString(fmt.Sprintf("data:%s\n", data.Value))
		}
	}
	if id != "" {
		eventMsg.WriteString(fmt.Sprintf("id:%s\n", strings.Replace(id, "\n", "", -1)))
	}
	eventMsg.WriteString("\n")
	return eventMsg.String()
}

// A eventer represents a interface events
type eventer interface {
	dispatch(*mpConsumer)
}

// A Event represents an event to send all consumers
type Event struct {
	eventer
	Event string
	Data  *DataEvent
	ID    string
}

// dispatch sends event all consumers
func (e *Event) dispatch(cons *mpConsumer) {
	msg := formattingEvent(e.Event, *e.Data, e.ID)
	cons.RLock()
	defer cons.RUnlock()
	for _, consumer := range cons.value {
		consumer.mainChannel <- msg
	}
}

// A EventOnly represents an event to send only clients with UCID
type EventOnly struct {
	eventer
	CID   []interface{}
	Event string
	Data  *DataEvent
	ID    string
}

// dispatch sends event only clients with CID
func (e *EventOnly) dispatch(cons *mpConsumer) {
	msg := formattingEvent(e.Event, *e.Data, e.ID)
	cons.RLock()
	defer cons.RUnlock()
	for _, CID := range e.CID {
		if consumer, ok := cons.value[CID]; ok {
			consumer.mainChannel <- msg
		}
	}
}

// A EventExcept represents an event to send except clients with CID
type EventExcept struct {
	eventer
	CID   []interface{}
	Event string
	Data  *DataEvent
	ID    string
}

// isValue checks exist value in list
func (e *EventExcept) isValue(value interface{}, list []interface{}) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// dispatch sends event only except clients with CID
func (e *EventExcept) dispatch(cons *mpConsumer) {
	msg := formattingEvent(e.Event, *e.Data, e.ID)
	cons.RLock()
	defer cons.RUnlock()
	for CID, consumer := range cons.value {
		if !e.isValue(CID, e.CID) {
			consumer.mainChannel <- msg
		}
	}
}

// A EventRecovery represents an priority event to send only one client with CID
// which there are fulfilled conditions open recovery channel.
type EventRecovery struct {
	eventer
	CID   interface{}
	Event string
	Data  *DataEvent
	ID    string
}

// dispatch sends priority event to send only one client with CID
func (e *EventRecovery) dispatch(cons *mpConsumer) {
	msg := formattingEvent(e.Event, *e.Data, e.ID)
	cons.RLock()
	defer cons.RUnlock()
	if consumer, ok := cons.value[e.CID]; ok {
		consumer.recoveryChannel <- msg
	}
}

// A EventRetry represents an event to send time in seconds which it means time
// waitting reconnecting consumer to server
type EventRetry struct {
	eventer
	Time time.Duration
}

// dispatch sends event to send all consumers
func (e *EventRetry) dispatch(cons *mpConsumer) {
	msg := fmt.Sprintf("retry:%d\n\n", e.Time/time.Millisecond)
	cons.RLock()
	defer cons.RUnlock()
	for _, consumer := range cons.value {
		consumer.mainChannel <- msg
	}
}
