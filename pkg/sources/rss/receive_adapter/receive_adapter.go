package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/knative/eventing/pkg/event"
	"github.com/mmcdole/gofeed"
	"time"
	"errors"
)

const (
	envTarget = "TARGET"
	envFeedURL = "FEED_URL"
	envWatermarkTime = "WATERMARK_TIME"
	timeLayout = "%Y-%m-%d %H:%M:%sz"
)

type RssAdapter struct {
	target string
	watermark *time.Time
	feedParser *gofeed.Parser
}

func main() {
	flag.Parse()

	target := os.Getenv(envTarget)
	feedURL := os.Getenv(envFeedURL)
	watermark, err := time.Parse(timeLayout, os.Getenv(envWatermarkTime))
	if err != nil {
		log.Fatalf("Error parsing the watermark time: %v :: %v", err, os.Getenv(envWatermarkTime))
		return
	}


	fp := gofeed.NewParser()
	rssAdapter := RssAdapter{target: target, watermark: &watermark, feedParser: fp}

	err = rssAdapter.parseAndPublish(feedURL)
	if err != nil {
		log.Fatalf("Error parsing and publishing the feed: %v :: %v", feedURL, err)
	}

	err = updateWatermark()
	if err != nil {
		log.Fatalf("Error updating the watermark: %v", err)
	}
}

func (r *RssAdapter) parseAndPublish(feedURL string) error {
	feed, err := r.feedParser.ParseURL(feedURL)
	if err != nil {
		return err
	}

	for _, item := range feed.Items {
		if r.newerThanWatermark(item) {
			err := r.publish(feedURL, item)
			if err != nil {
				// Not publishing a single message is not fatal. However, only the last error will
				// be returned.
				log.Printf("Error publishing the item. %v :: %v", err, item)
				continue
			}
		}
	}
	return err
}

func (r *RssAdapter) newerThanWatermark(item *gofeed.Item) bool {
	itemTime, err := getItemTime(item)
	if err != nil {
		// If we can't determine when this item is from, just assume it is new.
		return true
	}
	return r.watermark.Before(itemTime)
}

func getItemTime(item *gofeed.Item) (time.Time, error) {
	if item.UpdatedParsed != nil {
		return *item.UpdatedParsed, nil
	} else if item.PublishedParsed != nil {
		return *item.PublishedParsed, nil
	}
	return time.Time{}, errors.New("unable to find a time for the item")
}


func (r *RssAdapter) publish(feedURL string, item *gofeed.Item) error {
	URL := fmt.Sprintf("http://%s/", r.target)
	itemTime, err := getItemTime(item)
	if err != nil {
		// Just pretend it is the current time.
		itemTime = time.Now()
	}
	ctx := event.EventContext{
		CloudEventsVersion: event.CloudEventsVersion,
		EventType:	"dev.knative.rss.event",
		EventID:	item.GUID,
		EventTime:	itemTime,
		Source:		feedURL,
	}
	req, err := event.Binary.NewRequest(URL, item, ctx)
	if err != nil {
		log.Printf("Failed to marshal the message: %+v : %s", item, err)
		return err
	}

	log.Printf("Posting to %q", URL)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	log.Printf("response Status: %s", resp.Status)
	body, _ := ioutil.ReadAll(resp.Body)
	log.Printf("response Body: %s", string(body))
	return nil
}

func updateWatermark() error {
	return errors.New("not yet implemented")
}
