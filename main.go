package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/fhs/gompd/mpd"
)

const statusMaxLength = 100

var lastSong string

type SlackClient struct {
	apiToken, apiUrl, versionUid string
	defaultEmoji, defaultText    string
}

func (slack *SlackClient) call(method string, args url.Values) error {
	args.Add("token", slack.apiToken)

	timestamp := time.Now().Unix()
	uri := slack.apiUrl + method + "?_x_id=" + slack.versionUid + "-" + strconv.FormatInt(timestamp, 10)

	_, err := http.PostForm(uri, args)
	return err
}

func setStatus(slack *SlackClient, emoji, text string) error {
	if len(text) > statusMaxLength {
		text = text[:statusMaxLength-2] + "…"
	}

	log.Printf("Setting status [%s] %s\n", emoji, text)

	payload, _ := json.Marshal(map[string]string{
		"status_text":  text,
		"status_emoji": emoji,
	})

	return slack.call("users.profile.set", url.Values{
		"profile": {string(payload)},
	})
}

func isPlaying(client *mpd.Client) bool {
	attrs, err := client.Status()
	if err != nil {
		return false
	}

	return attrs["state"] == "play"
}

func setCurrentSongStatus(slack *SlackClient, client *mpd.Client) error {
	attrs, err := client.CurrentSong()
	if err != nil {
		return err
	}

	song := attrs["Title"] + " - " + attrs["Artist"]

	if song == lastSong {
		return nil
	}
	lastSong = song

	return setStatus(slack, ":headphones:", song)
}

func resetStatus(slack *SlackClient) error {
	return setStatus(slack, slack.defaultEmoji, slack.defaultText)
}

func start(slack *SlackClient, client *mpd.Client, watcher *mpd.Watcher) error {
	var err error
	for _ = range watcher.Event {
		if isPlaying(client) {
			err = setCurrentSongStatus(slack, client)
		} else {
			err = resetStatus(slack)
		}

		if err != nil {
			break
		}
	}

	return err
}

func main() {
	var (
		apiToken     = flag.String("api-token", "", "Your Slack API token")
		apiUrl       = flag.String("api-url", "", "Full URL to API path for the Slack team")
		versionUid   = flag.String("version-uid", "", "The Slack version uid")
		mpdNetwork   = flag.String("mpd-network", "tcp", "")
		mpdAddress   = flag.String("mpd-address", ":6600", "")
		defaultEmoji = flag.String("default-emoji", ":question:", "")
		defaultText  = flag.String("default-text", "I don't know", "")
	)
	flag.Parse()

	slack := &SlackClient{
		apiToken:     *apiToken,
		apiUrl:       *apiUrl,
		versionUid:   *versionUid,
		defaultEmoji: *defaultEmoji,
		defaultText:  *defaultText,
	}

	client, err := mpd.Dial(*mpdNetwork, *mpdAddress)
	if err != nil {
		log.Println("Failed to connect to mpd")
		log.Fatal(err)
	}
	defer client.Close()

	go func() {
		for _ = range time.Tick(30 * time.Second) {
			client.Ping()
		}
	}()

	watcher, err := mpd.NewWatcher(*mpdNetwork, *mpdAddress, "", "player")
	if err != nil {
		log.Println("Failed to create mpd watcher")
		log.Fatal(err)
	}
	defer watcher.Close()

	if err := start(slack, client, watcher); err != nil {
		log.Fatal(err)
	}
}
