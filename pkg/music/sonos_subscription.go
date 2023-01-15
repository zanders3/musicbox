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

const Lifetime = 20 * time.Second

type Subscriptions struct {
	internalIpAddr string
	subs           map[string]func(e string)
	subsMu         sync.Mutex
}

func ListenForSubscriptionEvents(internalIpAddr string) *Subscriptions {
	s := &Subscriptions{subs: make(map[string]func(e string)), internalIpAddr: internalIpAddr}
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
				if sub, ok := s.subs[sid]; ok {
					sub(d.Properties[0].Result.Value)
				}
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
			s.subs[sid] = gotEvent
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
					log.Printf("failed to unsub %s: %s", eventUrl.String(), err.Error())
					return
				}
				log.Printf("unsubscribed ok!")
				return
			}
		}
	}()
}
