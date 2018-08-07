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
	watermark, err := time.Parse(time.RFC3339, os.Getenv(envWatermarkTime))
	if err != nil {
		log.Fatalf("Error parsing the watermark time: %v :: %v", err, os.Getenv(envWatermarkTime))
		return
	}


	fp := gofeed.NewParser()
	rssAdapter := RssAdapter{target: target, watermark: &watermark, feedParser: fp}

	newWatermark, err := rssAdapter.parseAndPublish(feedURL)
	if err != nil {
		log.Fatalf("Error parsing and publishing the feed: %v :: %v", feedURL, err)
	}

	err = updateWatermark(newWatermark)
	if err != nil {
		log.Fatalf("Error updating the watermark: %v", err)
	}
}

func (r *RssAdapter) parseAndPublish(feedURL string) (time.Time, error) {
	feed, err := r.feedParser.ParseURL(feedURL)
	if err != nil {
		return time.Time{}, err
	}

	var newWatermark time.Time
	for _, item := range feed.Items {
		if newerThanWatermark, itemTime := r.newerThanWatermark(item); newerThanWatermark {
			if itemTime.After(newWatermark) {
				newWatermark = itemTime
			}
			err := r.publish(feedURL, item)
			if err != nil {
				// Not publishing a single message is not fatal. However, only the last error will
				// be returned.
				log.Printf("Error publishing the item. %v :: %v", err, item)
				continue
			}
		}
	}
	return newWatermark, err
}

func (r *RssAdapter) newerThanWatermark(item *gofeed.Item) (bool, time.Time) {
	itemTime, err := getItemTime(item)
	if err != nil {
		// If we can't determine when this item is from, just assume it is new.
		return true, time.Time{}
	}
	return r.watermark.Before(itemTime), itemTime
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

func updateWatermark(newWatermark time.Time) error {
	_ = newWatermark.Format(time.RFC3339)
	return errors.New("not yet implemented")
}
