package gocomet

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

type MetaMessage struct {
	Channel                  string      `json:"channel"`
	Version                  string      `json:"version,omitempty"`
	MinimumVersion           string      `json:"minimumVersion,omitempty"`
	SupportedConnectionTypes []string    `json:"supportedConnectionTypes,omitempty"`
	ClientId                 string      `json:"clientId,omitempty"`
	Advice                   *Advice     `json:"advice,omitempty"`
	ConnectionType           string      `json:"connectionType,omitempty"`
	Id                       string      `json:"id,omitempty"`
	Timestamp                string      `json:"timestamp,omitempty"`
	Data                     string      `json:"data,omitempty"`
	Successful               bool        `json:"successful"`
	Subscription             string      `json:"subscription,omitempty"`
	Error                    string      `json:"error,omitempty"`
	Extension                interface{} `json:"ext,omitempty"`
}

type EventMessage struct {
	Channel   string      `json:"channel"`
	Data      string      `json:"data"`
	Id        string      `json:"id,omitempty"`
	ClientId  string      `json:"clientId,omitempty"`
	Extension interface{} `json:"ext,omitempty"`
	Advice    *Advice     `json:"advice,omitempty"`
}

func (mm *MetaMessage) String() string {
	switch mm.Channel {
	case "/meta/handshake":
		return fmt.Sprintf("Handshake:%v:%v", mm.Version, strings.Join(mm.SupportedConnectionTypes, ","))
	case "/meta/connect":
		return fmt.Sprintf("Connect:%v:%v", mm.ClientId, mm.ConnectionType)
	case "/meta/disconnect":
		return fmt.Sprintf("Disconnect:%v", mm.ClientId)
	case "/meta/subscribe":
		return fmt.Sprintf("Subscribe:%v:%v", mm.ClientId, mm.Subscription)
	case "/meta/unsubscribe":
		return fmt.Sprintf("Unsubscribe:%v:%v", mm.ClientId, mm.Subscription)
	default:
		switch {
		case mm.Data != "":
			return fmt.Sprintf("%v:%v:%v", mm.Channel, mm.ClientId, mm.Data)
		default:
			return fmt.Sprintf("Invalid:%v", mm)
		}
	}
}

type Advice struct {
	Reconnect string `json:"reconnect,omitempty"`
	Timeout   int64  `json:"timeout,omitempty"`
	Interval  int    `json:"interval,omitempty"`
}

const (
	VERSION          = "1.0"
	MINIMUM_VERSION  = "1.0"
	DEFAULT_INTERVAL = 0
)

type Instance struct {
	*Server
	services map[string]func(session *Session, message *MetaMessage)
}

/*
Create a simple cometd instace.
*/
func New() *Instance {
	return &Instance{
		Server:   newServer(),
		services: make(map[string]func(session *Session, message *MetaMessage)),
	}
}

func (inst *Instance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if r.Method != "POST" {
		http.Error(w, "Long-Polling only supports POST method.", http.StatusBadRequest)
		return
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var messages []*MetaMessage
	err = json.Unmarshal(data, &messages)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(messages) == 0 {
		http.Error(w, "Found no message.", http.StatusBadRequest)
		return
	}
	data = nil
	// log.Printf("Received requests: %v", messages)

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.WriteHeader(http.StatusOK)

	var responses []*MetaMessage
	var allEvents []chan *Message
	var waiting chan *Message
	var timeout chan bool // notify uptream chanel to stop
	var clientId string   // client ID for connect message
	for _, message := range messages {
		var events chan *Message
		var ok bool
		var response = &MetaMessage{}
		switch message.Channel {
		case "/meta/handshake":
			log.Println("Handshaking...")
			response.Channel = "/meta/handshake"
			response.Id = message.Id
			response.Advice = &Advice{
				Reconnect: "retry",
				Interval:  DEFAULT_INTERVAL,
				Timeout:   1000 * int64(MAX_SESSION_IDEL.Seconds()),
			}
			if clientId, err := inst.handshake(); err == nil {
				response.Version = VERSION
				response.SupportedConnectionTypes = []string{"long-polling"}
				response.ClientId = clientId
				response.Successful = true
			} else {
				response.Error = err.Error()
			}
		case "/meta/connect":
			log.Printf("[%8.8v]Connecting...", message.ClientId)
			response.Channel = "/meta/connect"
			response.ClientId = message.ClientId
			response.Id = message.Id
			var ch chan bool
			if events, ch, ok = inst.connect(message.ClientId); ok && waiting == nil {
				// only one connect message is allowed
				clientId = message.ClientId
				waiting, timeout = events, ch
				response.Successful = true
				response.Advice = &Advice{
					Reconnect: "retry",
					Interval:  DEFAULT_INTERVAL,
					Timeout:   1000 * int64(MAX_SESSION_IDEL.Seconds()),
				}
			} else {
				log.Printf("[%8.8v]Client ID not found.", message.ClientId)
				response.Advice = &Advice{
					Reconnect: "handshake",
					Interval:  DEFAULT_INTERVAL,
					Timeout:   1000 * int64(MAX_SESSION_IDEL.Seconds()),
				}
			}
		case "/meta/disconnect":
			response.Channel = "/meta/disconnect"
			response.ClientId = message.ClientId
			response.Id = message.Id
			if events, ok = inst.disconnect(message.ClientId); ok {
				allEvents = append(allEvents, events)
				response.Successful = true
			}
		case "/meta/subscribe":
			log.Printf("[%8.8v]Subscribing to %v...", message.ClientId, message.Subscription)
			response.Channel = "/meta/subscribe"
			response.ClientId = message.ClientId
			response.Subscription = message.Subscription
			response.Id = message.Id
			if events, ok = inst.subscribe(message.ClientId, message.Subscription); ok {
				log.Printf("[%8.8v]success.", message.ClientId)
				allEvents = append(allEvents, events)
				response.Successful = true
			} else {
				log.Printf("[%8.8v]fail.", message.ClientId)
			}
		case "/meta/unsubscribe":
			response.Channel = "/meta/unsubscribe"
			response.ClientId = message.ClientId
			response.Subscription = message.Subscription
			response.Id = message.Id
			if events, ok = inst.unsubscribe(message.ClientId, message.Subscription); ok {
				allEvents = append(allEvents, events)
				response.Successful = true
			}
		default:
			if message.Data != "" { // publish
				response.Channel = message.Channel
				response.Id = message.Id
				if message.ClientId == "" { // whisper
					log.Printf("Whispering '%v' to '%v'...", message.Data, message.Channel)
					inst.whisper(message.Channel, message.Data)
					response.Successful = true
				} else if events, ok = inst.publish(message.ClientId, message.Channel, message.Data); ok {
					allEvents = append(allEvents, events)
					response.Successful = true
				}
			} else { // invalid requests
				response.Channel = message.Channel
				response.Id = message.Id
				response.Successful = false
				response.Error = fmt.Sprintf("400:%v:Bad request", message.Channel)
			}
		}
		responses = append(responses, response)
	}
	messages = nil

	var events []*Message
	if waiting != nil { // it's a connect message
		var event *Message
		var remaining = start.Add(MAX_SESSION_IDEL / 2).Sub(time.Now())
		log.Printf("[%8.8v]Listening to %v for %v seconds...", clientId, waiting, remaining.Seconds())
		var isDone = false
		// wait for at least one event first
		select {
		case event = <-waiting:
			if event != nil { // waiting channel is closed already?
				events = append(events, event)
			}
		case <-time.After(remaining):
			// timeout and should return immediately
			timeout <- true
			isDone = true
		}

		// wait for another second to see if other events come
		// otherwise, notify the upstream channel to stop sending more
		// but no more than half of the max idle time
		var renew = make(chan bool)
		go func(isWaiting bool) {
			for isWaiting {
				remaining := start.Add(MAX_SESSION_IDEL / 2).Sub(time.Now())
				log.Printf("[%8.8v]Listening to %v for %v seconds...", clientId, waiting, remaining.Seconds())
				select {
				case <-time.After(remaining):
					timeout <- true
					isWaiting = false
				case <-time.After(1 * time.Second):
					timeout <- true
					isWaiting = false
				case <-renew:
					// do nothing
				}
			}
		}(!isDone)

		for event := range waiting {
			events = append(events, event)
			if !isDone {
				renew <- true
			}
		}
		log.Printf("[%8.8v]%v events collected.", clientId, len(events))
	}

	fmt.Fprintf(w, "[")
	if len(events) > 0 {
		log.Printf("[%8.8v]Collected %v event messages.", clientId, len(events))
		for _, event := range events {
			data, _ = json.Marshal(&EventMessage{
				Channel: event.channel,
				Data:    event.data,
			})
			fmt.Fprintf(w, "%s,", data)
		}
	}
	for _, resp := range responses[:len(responses)-1] {
		data, _ = json.Marshal(resp)
		fmt.Fprintf(w, "%s,", data)
	}
	data, _ = json.Marshal(responses[len(responses)-1])
	fmt.Fprintf(w, "%s]", data)
	log.Printf("[%8.8v]Request is processd.", clientId)
}

/*
Add new handler to listen and process messages sent to /service/**
channel. It doesn't check for conflict and will override existing one
with the same name. The returned Instance object allows flow style
configuration.
*/
func (c *Instance) AddService(channel string, handler func(session *Session, message *MetaMessage)) *Instance {
	c.services[channel] = handler
	return c
}
