package music

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type EventData struct {
	Properties []Property `xml:"property"`
}

type Property struct {
	Result Result `xml:",any"`
}

type Result struct {
	Value string `xml:",chardata"`
}

const Lifetime = 60 * time.Second

type subscription struct {
	maxId          int
	callbacks      []subscriptionCallback
	shutdown       chan struct{}
	sid, lastEvent string
}

type subscriptionCallback struct {
	id       int
	gotEvent func(e string)
}

type Subscriptions struct {
	internalIpAddr string
	subs           map[string]*subscription
	subsMu         sync.Mutex
}

func ListenForSubscriptionEvents(internalIpAddr string) *Subscriptions {
	s := &Subscriptions{subs: make(map[string]*subscription), internalIpAddr: internalIpAddr}
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)

			b, err := io.ReadAll(r.Body)
			if err != nil {
				log.Printf("ERROR - Failed to read notification body: %v", err)
				return
			}
			d := EventData{}
			if err = xml.Unmarshal(b, &d); err != nil {
				log.Printf("ERROR - Unmarshal(): %v", err)
				return
			}
			sid := r.Header.Get("SID")
			if len(d.Properties) > 0 {
				s.subsMu.Lock()
				for _, sub := range s.subs {
					if sub.sid == sid {
						sub.lastEvent = d.Properties[0].Result.Value
						for _, cb := range sub.callbacks {
							cb.gotEvent(sub.lastEvent)
						}
						break
					}
				}
				s.subsMu.Unlock()
			}
		})
		if err := http.ListenAndServe(internalIpAddr+":3001", mux); err != nil {
			panic(err)
		}
	}()
	return s
}

func sendSubscribeRequest(eventUrl *url.URL, internalIpAddr string, sid string) (string, time.Duration, error) {
	req, err := http.NewRequest("SUBSCRIBE", eventUrl.String(), http.NoBody)
	if err != nil {
		return "", time.Duration(0), err
	}
	req.Header.Set("TIMEOUT", fmt.Sprintf("Second-%d", int(Lifetime.Seconds())))
	if len(sid) < 1 {
		req.Header.Set("NT", "upnp:event")
		req.Header.Set("CALLBACK", fmt.Sprintf("<http://%s:3001/>", internalIpAddr))
	} else {
		req.Header.Set("SID", sid)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", time.Duration(0), err
	}
	if res.StatusCode != 200 {
		return "", time.Duration(0), fmt.Errorf("SUBSCRIBE request returned HTTP %d", res.StatusCode)
	}
	tmout := -1
	h := res.Header.Get("TIMEOUT")
	if len(h) > 0 {
		t := strings.Split(h, "-")
		tmout, _ = strconv.Atoi(t[1])
	}
	return res.Header.Get("SID"), time.Duration(tmout) * time.Second, nil
}

func (s *Subscriptions) Subscribe(eventUrl *url.URL, unsubscribe chan struct{}, gotEvent func(e string)) {
	s.subsMu.Lock()
	sub, ok := s.subs[eventUrl.String()]
	var cbid int
	var lastEvent string
	if ok {
		sub.maxId++
		cbid = sub.maxId
		sub.callbacks = append(sub.callbacks, subscriptionCallback{id: sub.maxId, gotEvent: gotEvent})
		lastEvent = sub.lastEvent
	} else {
		cbid = 1
		sub = &subscription{
			callbacks: []subscriptionCallback{{id: 1, gotEvent: gotEvent}},
			shutdown:  make(chan struct{}),
			maxId:     1,
		}
		s.subs[eventUrl.String()] = sub

		go func() {
			var sid string
			for {
				log.Printf("subscribing %s with sid=%s", eventUrl.String(), sid)
				var err error
				var timeout time.Duration
				sid, timeout, err = sendSubscribeRequest(eventUrl, s.internalIpAddr, sid)
				if err != nil {
					log.Printf("subscribe to %s failed: %s", eventUrl.String(), err.Error())
					return
				}
				log.Printf("%s subscribe succeeded with sid %s and timeout %s", eventUrl.String(), sid, timeout.String())
				s.subsMu.Lock()
				s.subs[eventUrl.String()].sid = sid
				s.subsMu.Unlock()

				select {
				case <-time.After(timeout - 4*time.Second):
				case <-unsubscribe:
					log.Printf("unsubscribing %s", eventUrl.String())
					req, err := http.NewRequest("UNSUBSCRIBE", eventUrl.String(), http.NoBody)
					if err != nil {
						log.Println(err.Error())
						return
					}
					req.Header.Set("SID", sid)
					res, err := http.DefaultClient.Do(req)
					if err != nil {
						log.Printf("failed to unsub %s: %s", eventUrl.String(), err.Error())
						return
					}
					if res.StatusCode != 200 {
						log.Printf("failed to unsub %s", eventUrl.String())
						return
					}
					log.Printf("unsubscribed ok!")
					return
				}
			}
		}()
	}
	s.subsMu.Unlock()
	if len(lastEvent) > 0 {
		gotEvent(lastEvent)
	}
	go func(cbid int) {
		<-unsubscribe
		s.subsMu.Lock()
		defer s.subsMu.Unlock()
		if sub, ok := s.subs[eventUrl.String()]; ok {
			newCbs := make([]subscriptionCallback, 0)
			for _, cb := range sub.callbacks {
				if cb.id == cbid {
					continue
				} else {
					newCbs = append(newCbs, cb)
				}
			}
			sub.callbacks = newCbs
			if len(sub.callbacks) == 0 {
				close(sub.shutdown)
				delete(s.subs, eventUrl.String())
			}
		}
	}(cbid)
}
