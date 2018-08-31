package main

import (
	"fmt"
	"log"
	"time"
	"regexp"
	"os"		// for Encoder

	"google.golang.org/api/youtube/v3"
	"github.com/BurntSushi/toml"
)

type tomlKnownVideos struct {
	Videos map[string]videoMeta
}

type videoMeta struct {
  VideoId string
  Title string
  Published time.Time // requires `import time`
  Duration time.Duration
}

// Retrieve playlistItems in the specified playlist
func playlistItemsList(service *youtube.Service, part string, playlistId string, pageToken string) *youtube.PlaylistItemListResponse {
	call := service.PlaylistItems.List(part)
	call = call.PlaylistId(playlistId)
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}
	response, err := call.Do()
	handleError(err, "")
	return response
}

// Retrieve resource for the authenticated user's channel
func channelsListMine(service *youtube.Service, part string) *youtube.ChannelListResponse {
	call := service.Channels.List(part)
	call = call.Mine(true)
	response, err := call.Do()
	handleError(err, "")
	return response
}

func myVideos(knownVideos *tomlKnownVideos) []youtube.PlaylistItem {

	playlist := make([]youtube.PlaylistItem, 1)
	client := getClient(youtube.YoutubeReadonlyScope)
	service, err := youtube.New(client)
	
	// videoMeta data does not exist if there is no local data in knownvideos.toml
	if knownVideos.Videos == nil {
		knownVideos.Videos = make(map[string]videoMeta)
	}
	if err != nil {
		log.Fatalf("Error creating YouTube client: %v", err)
	}

	response := channelsListMine(service, "contentDetails")

	for _, channel := range response.Items {
		playlistId := channel.ContentDetails.RelatedPlaylists.Uploads
		
		// Print the playlist ID for the list of uploaded videos.
		fmt.Printf("Videos in list %s\r\n", playlistId)

		nextPageToken := ""
		for {
			// Retrieve next set of items in the playlist.
			playlistResponse := playlistItemsList(service, "snippet,ContentDetails", playlistId, nextPageToken)
			
			for _, playlistItem := range playlistResponse.Items {
				playlist = append(playlist, *playlistItem)
				// Thanks to https://github.com/go-shadow/moment/blob/master/moment.go for the format that must be used
				// https://golang.org/src/time/format.go?s=37668:37714#L735
				vidPublishTime, err := time.Parse("2006-01-02T15:04:05Z0700",playlistItem.ContentDetails.VideoPublishedAt)
				check(err)
				vidDuration, err := time.ParseDuration("0ms")		// in case they ever make this data available
				check(err)

				// Save video information into knownVideos so we can save it to disk
				// after getting the duration, which is actually the whole point of this thing.
				// Eventually we will ask knownVideos if the information is already known
				// and use that to decide if we keep going through pages of videos
				knownVideos.Videos[playlistItem.Snippet.ResourceId.VideoId] =
					videoMeta{
						VideoId:playlistItem.Snippet.ResourceId.VideoId,
						Title:playlistItem.Snippet.Title,
						Published:vidPublishTime,
						Duration:vidDuration,
					}
			}

			// Set the token to retrieve the next page of results
			// or exit the loop if all results have been retrieved.
			nextPageToken = playlistResponse.NextPageToken
			if nextPageToken == "" {
				break
			}
		}
	}
	return playlist
}

func check(e error) {
    if e != nil {
        panic(e)
    }
}

func main() {
	var knownVideos tomlKnownVideos		// knownVideos will be read from local TOML file

	_, err := toml.DecodeFile("/Users/thunderrabbit/mt3.com/data/playlists/livestreams/knownvideos.toml", &knownVideos)
	check(err)

	fmt.Print(knownVideos)

	playlist := myVideos(&knownVideos)


	// For more granular writes, open a file for writing.
	f, err := os.Create("/Users/thunderrabbit/mt3.com/data/playlists/livestreams/knownvideos.toml")
	check(err)

	// It's idiomatic to defer a `Close` immediately
	// after opening a file.
	defer f.Close()

	err = toml.NewEncoder(f).Encode(knownVideos);
	check(err)

	for _, video := range playlist {
		// sometimes Snippets are nil but I am not sure why
		if video.Snippet != nil {
			videoId := video.Snippet.ResourceId.VideoId
			title := video.Snippet.Title
			match, _ := regexp.MatchString("Live Stream:", title)
			if match {
				fmt.Printf("%v  \"%v\" %v \r\n", videoId, title, match)
			} else {
				fmt.Printf("%v skipped\r\n", title)
			}
		}
	}
}
