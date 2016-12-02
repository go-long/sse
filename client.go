/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package sse

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"log"
	"net/http"
	"strings"

)

var (
	headerID    = []byte("id:")
	headerData  = []byte("data:")
	headerEvent = []byte("event:")
	headerError = []byte("error:")
)

// Client handles an incoming server stream
type Client struct {
	URL            string
	Connection     *http.Client
	Headers        map[string]string
	EncodingBase64 bool
}

// NewClient creates a new client
func NewClient(url string) *Client {
	return &Client{
		URL:        url,
		Connection: &http.Client{},
		Headers:    make(map[string]string),
	}
}

// Subscribe to a data stream
func (c *Client) Subscribe(stream string, handler func(msg []byte)) error {
	resp, err := c.request(stream)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

	for {
		// Read each new line and process the type of event
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return err

		}

		if len(line)>1 {
			handler(line)
		}

		//msg := c.processEvent(line)
		//if msg != nil {
		//	handler(msg)
		//}
	}
}

// Subscribe to a data stream
func (c *Client) SubscribeEvent(stream string, handler func(msg *Event)) error {
	resp, err := c.request(stream)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	msg := &Event{

		Data:&DataEvent{},
	}
	for {
		// Read each new line and process the type of event
		line, err := reader.ReadBytes('\n')
		//fmt.Println("line:",string(line),len(line))
		if err != nil {
			return err
		}
		if len(strings.TrimSpace(string(line)))==0 {
			//fmt.Println("eeerrr:")
			handler(msg)

			msg = &Event{
				Data:&DataEvent{},
			}
		}

		c.processEvent(line,msg)
	}

}

// SubscribeChan sends all events to the provided channel
func (c *Client) SubscribeChan(stream string, ch chan []byte) error {
	resp, err := c.request(stream)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)


	for {
		// Read each new line and process the type of event
		line, err := reader.ReadBytes('\n')
		if err != nil {
			close(ch)
			return err
		}
		ch <- line

		//msg := c.processEvent(line)
		//if msg != nil {
		//	ch <- msg
		//}
	}
}

func (c *Client) request(stream string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.URL, nil)
	if err != nil {
		return nil, err
	}

	// Setup request, specify stream to connect to
	query := req.URL.Query()
	query.Add("stream", stream)
	req.URL.RawQuery = query.Encode()

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Connection", "keep-alive")

	// Add user specified headers
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}

	return c.Connection.Do(req)
}

func (c *Client) processEvent(msg []byte,e *Event)  {


	switch h := msg; {
	case bytes.Contains(h, headerID):
		e.ID = string(trimHeader(len(headerID), msg))
	case bytes.Contains(h, headerData):
		e.Data = &DataEvent{
			Value:string(trimHeader(len(headerData), msg)),
			DisabledFormatting:false,
		}
	case bytes.Contains(h, headerEvent):
		e.Event = string(trimHeader(len(headerEvent), msg))
	case bytes.Contains(h, headerError):
		e.Error = string(trimHeader(len(headerError), msg))
	default:
		return
	}


	if len(e.Data.Value) > 0 && c.EncodingBase64 {
		buf := make([]byte, base64.StdEncoding.DecodedLen(len(e.Data.Value)))

		_, err := base64.StdEncoding.Decode(buf, []byte(e.Data.Value))
		if err != nil {
			log.Println(err)
		}

		e.Data.Value = string(buf)
	}

	return
}

//
//func (c *Client) processEvent(msg []byte) *Event {
//	var e Event
//
//	switch h := msg; {
//	case bytes.Contains(h, headerID):
//		e.ID = string(trimHeader(len(headerID), msg))
//	case bytes.Contains(h, headerData):
//		e.Data =
//			trimHeader(len(headerData), msg)
//	case bytes.Contains(h, headerEvent):
//		e.Event = string(trimHeader(len(headerEvent), msg))
//	case bytes.Contains(h, headerError):
//		e.Error = string(trimHeader(len(headerError), msg))
//	default:
//		return nil
//	}
//
//	if len(e.Data) > 0 && c.EncodingBase64 {
//		buf := make([]byte, base64.StdEncoding.DecodedLen(len(e.Data)))
//
//		_, err := base64.StdEncoding.Decode(buf, e.Data)
//		if err != nil {
//			log.Println(err)
//		}
//
//		e.Data = buf
//	}
//
//	return &e
//}

func trimHeader(size int, data []byte) []byte {
	data = data[size:]
	// Remove optional leading whitespace
	if data[0] == 32 {
		data = data[1:]
	}
	// Remove trailing new line
	if data[len(data)-1] == 10 {
		data = data[:len(data)-1]
	}
	return data
}
