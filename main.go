package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gempir/go-twitch-irc/v2"
)

func main() {
	token, exists := os.LookupEnv("JONBOT_TWITCH_TOKEN")
	if !exists {
		log.Fatal("JONBOT_TWITCH_TOKEN is required")
	}

	username, exists := os.LookupEnv("JONBOT_TWITCH_USERNAME")
	if !exists {
		log.Fatal("JONBOT_TWITCH_USERNAME is required")
	}

	interval, exists := os.LookupEnv("JONBOT_TWITCH_TIMERINTERVAL")
	if !exists {
		interval = "5m"
	}

	timerInterval, err := time.ParseDuration(interval)
	if err != nil {
		log.Fatal(err)
	}

	channelsEnv := os.Getenv("JONBOT_TWITCH_CHANNELS")
	channels := strings.Split(channelsEnv, ",")

	client := twitch.NewClient(username, token)

	s := &state{
		client:        client,
		channels:      make(map[string]*channel),
		username:      username,
		timerInterval: timerInterval,
	}

	client.Join(channels...)
	client.OnPrivateMessage(s.handlePrivateMessage())

	err = client.Connect()
	if err != nil {
		log.Fatal(err)
	}
}

func (s *state) handlePrivateMessage() func(msg twitch.PrivateMessage) {
	return func(msg twitch.PrivateMessage) {
		splitted := strings.Split(msg.Message, " ")
		if len(splitted) == 0 {
			return
		}
		if splitted[0] != "@"+s.username && splitted[0] != "@idiot" {
			return
		}

		if len(splitted) == 1 {
			s.client.Say(msg.Channel, fmt.Sprintf("jonowGUN @%s", msg.User.DisplayName))
			return
		}

		channel := s.getOrCreateChannel(msg.Channel)
		switch splitted[1] {
		case "addtime", "ugh":
			channel.addTime(msg)
			return
		case "howmuchlonger", "timeleft", "remaining?":
			channel.timeLeft(msg)
			return
		case "uptime":
			channel.upFor(msg)
			return
		default:
			s.client.Say(msg.Channel, fmt.Sprintf("bruh ? @%s", msg.User.DisplayName))
			return
		}
	}
}

func chatTS(msg twitch.PrivateMessage) time.Time {
	t, ok := msg.Tags["tmi-sent-ts"]
	if !ok {
		return time.Now()
	}
	u, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return time.Now()
	}
	return time.Unix(u/1000, 0)
}

func (s *state) reply(m twitch.PrivateMessage, msg string) {
	s.client.Say(m.Channel, "@"+m.User.DisplayName+": "+msg)
}

func (s *state) getOrCreateChannel(name string) *channel {
	var c *channel
	var ok bool
	func() {
		s.m.RLock()
		defer s.m.RUnlock()
		c, ok = s.channels[name]
	}()
	if !ok {
		func() {
			s.m.Lock()
			defer s.m.Unlock()
			c, ok = s.channels[name]
			if !ok {
				c = &channel{timer: &timer{}, s: s}
				s.channels[name] = c
			}
		}()
	}
	return c
}

type state struct {
	username      string
	client        *twitch.Client
	channels      map[string]*channel
	timerInterval time.Duration

	m sync.RWMutex
}

type channel struct {
	s     *state
	timer *timer
}

func (c *channel) upFor(msg twitch.PrivateMessage) {
	c.timer.m.RLock()
	c.timer.m.RUnlock()
	if !c.timer.HasStarted() {
		c.s.reply(msg, "timer hasnt been initialized yet!")
		return
	}
	c.s.reply(msg, c.timer.UpFor(chatTS(msg)))
}

func (c *channel) timeLeft(msg twitch.PrivateMessage) {
	c.timer.m.RLock()
	c.timer.m.RUnlock()
	if !c.timer.HasStarted() {
		c.s.reply(msg, "timer hasnt been initialized yet!")
		return
	}
	c.s.reply(msg, c.timer.TimeLeft())
}

func (c *channel) addTime(msg twitch.PrivateMessage) {
	splitted := strings.Split(msg.Message, " ")
	if len(splitted) > 3 {
		c.s.reply(msg, "bruh it's 'addtime [# of intervals]'")
		return
	}
	var intervals int64
	if len(splitted) == 2 {
		intervals = 1
	} else {
		var err error
		intervals, err = strconv.ParseInt(splitted[2], 10, 64)
		if err != nil {
			c.s.reply(msg, "yo [# of intervals] is a number cmonBruh")
			return
		}
	}

	func() {
		c.timer.m.Lock()
		defer c.timer.m.Unlock()
		if !c.timer.HasStarted() {
			c.timer.startTime = chatTS(msg)
		}

		c.timer.duration += (time.Duration(intervals) * c.s.timerInterval)
		c.s.reply(msg, c.timer.TimeLeft())
		return
	}()
}

type timer struct {
	m         sync.RWMutex
	startTime time.Time
	duration  time.Duration
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC822Z)
}

func formatDuration(d time.Duration) string {
	return d.Truncate(time.Second).String()
}

func (t *timer) HasStarted() bool {
	return !t.startTime.IsZero()
}

func (t *timer) UpFor(now time.Time) string {
	return formatDuration(now.Sub(t.startTime))
}

func (t *timer) EndTime() string {
	return formatTime(t.startTime.Add(t.duration))
}

func (t *timer) TimeLeft() string {
	return formatDuration(time.Until(t.startTime.Add(t.duration)))
}

func sentByBroadcaster(msg twitch.PrivateMessage) bool {
	userID, ok := msg.Tags["user-id"]
	if !ok {
		return false
	}

	roomID, ok := msg.Tags["room-id"]
	if !ok {
		return false
	}

	if userID == roomID && userID != "" {
		return true
	}
	return false
}

func sentByMod(msg twitch.PrivateMessage) bool {
	if msg.Tags["mod"] == "1" {
		return true
	}
	if msg.Tags["user-type"] == "mod" {
		return true
	}
	return false
}

func sentByAdmin(msg twitch.PrivateMessage) bool {
	if sentByBroadcaster(msg) {
		return true
	}

	if sentByMod(msg) {
		return true
	}
	return false
}
