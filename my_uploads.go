package main

import (
	"fmt"
	"log"
	"time"
	"strings"	// needed to create a string of video IDs, separated by commas
	// "regexp"		// will be needed to parse Titles when searching for "Live Stream:"
	"bytes"		// for debugging Encoder
	"os"		// for Encoder

	"google.golang.org/api/youtube/v3"
	"github.com/BurntSushi/toml"
)

const localPathToKnownVideosFile = "/Users/thunderrabbit/mt3.com/data/playlists/livestreams/knownvideos.toml"

type tomlKnownVideos struct {
	Videos map[string]videoMeta
}

type videoMeta struct {
  VideoId string
  Title string
  Published time.Time // requires `import time`
  Duration time.Duration
}

// from https://developers.google.com/youtube/v3/docs/videos/list
// temp code to examine response
func printVideosListResults(response *youtube.VideoListResponse) {
	for _, item := range response.Items {
		fmt.Println(item.Id, ":", item.ContentDetails.Duration)
	}
}

// from https://developers.google.com/youtube/v3/docs/videos/list
// Used ONLY to get the Durations of videos because https://issuetracker.google.com/issues/35170788
// Thanks https://stackoverflow.com/questions/15596753/youtube-api-v3-how-to-get-video-durations
func videosListMultipleIds(service *youtube.Service, part string, id string) *youtube.VideoListResponse {
	call := service.Videos.List(part)
	if id != "" {
		call = call.Id(id)
	}
	response, err := call.Do()
	handleError(err, "")
	return response
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

func addNewVideosToList(playlistItem *youtube.PlaylistItem, knownVideos *tomlKnownVideos) {
	// Thanks to https://github.com/go-shadow/moment/blob/master/moment.go for the format that must be used
	// https://golang.org/src/time/format.go?s=37668:37714#L735
	vidPublishTime, err := time.Parse("2006-01-02T15:04:05Z0700",playlistItem.ContentDetails.VideoPublishedAt)
	check(err)
	vidDuration, err := time.ParseDuration("0ms")		// TODO put actual number here if they ever make this data available https://issuetracker.google.com/issues/35170788
	check(err)

	// See if the video key we loaded from Youtube's API is already known to us
	_, exists := knownVideos.Videos[playlistItem.Snippet.ResourceId.VideoId]
	// Save video information into knownVideos only if it does not exist
	//    (if it exists, we would overwrite the duration with 0)
	if !exists {
		knownVideos.Videos[playlistItem.Snippet.ResourceId.VideoId] =
			videoMeta{
				VideoId:playlistItem.Snippet.ResourceId.VideoId,
				Title:playlistItem.Snippet.Title,
				Published:vidPublishTime,
				Duration:vidDuration,
			}
	}
}

func loadNewVideosFromMyChannel(knownVideos *tomlKnownVideos) {

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
				addNewVideosToList(playlistItem, knownVideos)
			}

			// Set the token to retrieve the next page of results
			// or exit the loop if all results have been retrieved.
			nextPageToken = playlistResponse.NextPageToken
			if nextPageToken == "" {
				break
			}
		}
	}
}

func check(e error) {
    if e != nil {
        panic(e)
    }
}

func tomlPrintKnownVids(knownVideos tomlKnownVideos) {
	buf := new(bytes.Buffer)
	err := toml.NewEncoder(buf).Encode(knownVideos)
	check(err)
	fmt.Println(buf.String())
}

func loadLocalKnownVideos() tomlKnownVideos {
	var knownVideos tomlKnownVideos			// knownVideos will be read from local TOML file

	_, err := toml.DecodeFile(localPathToKnownVideosFile, &knownVideos)
	check(err)

	return knownVideos
}

func saveLocalKnownVideos(knownVideos tomlKnownVideos) {
	// For more granular writes, open a file for writing.
	f, err := os.Create(localPathToKnownVideosFile)
	check(err)

	// It's idiomatic to defer a `Close` immediately
	// after opening a file.
	defer f.Close()

	err = toml.NewEncoder(f).Encode(knownVideos);
	check(err)
}

// returns string of comma-separated video IDs
func returnUpTo50VideosWithEmptyDuration(knownVideos *tomlKnownVideos) string {

	limit50 := 1     					//  Make sure we don't try to load too many at once
	videoIDs := make([]string,1)		// need to send the video IDs as a comma separated string; this might not work easily

	// look through all the known videos to find those without Duration
	// so we can load the duration from Youtube API in this lovely separate call
	for _, video := range knownVideos.Videos {
		if video.Duration == 0 {
			videoIDs = append(videoIDs, video.VideoId)		// add to list of IDs we must look up the duration
			limit50 += 1									// count toward 50 iff the video has no Duration yet
		}
		if limit50 == 50 {
			fmt.Print("I have 50 video Ids. Gotta go, bye!\r\n")
			break					// we can only do 50 at a time
		}
	}
	return strings.Join(videoIDs,",")
}

// This will fill in up to 50 videos at a time.  50 is the limit on how many videoIDs can be sent to get their metadata
func fillInDurations(knownVideos *tomlKnownVideos) {

	client := getClient(youtube.YoutubeReadonlyScope)
	service, err := youtube.New(client)
	check(err)

	videoIDs := returnUpTo50VideosWithEmptyDuration(knownVideos)

	// Call async function to load the metadata for these video IDs
	response := videosListMultipleIds(service, "contentDetails", videoIDs)
	printVideosListResults(response)

	for _, item := range response.Items {
		// Google returns a format like PT1H45M41S
		// For time.ParseDuration, we have to
		//    crop off the PT                     with [2:]
		//    change to lower case
		// to send something like this  1h45m41s
		vidDuration, err := time.ParseDuration(strings.ToLower(item.ContentDetails.Duration[2:]))
		check(err)

		// https://stackoverflow.com/a/17443950/194309
		// I wanted to do this		knownVideos.Videos[item.Id].Duration = item.ContentDetails.Duration
		// but that gives an error.   Have to do this
		vid := knownVideos.Videos[item.Id]
		vid.Duration = vidDuration
		knownVideos.Videos[item.Id] = vid
	}
}

func main() {
	knownVideos := loadLocalKnownVideos()

//	tomlPrintKnownVids(knownVideos)

	loadNewVideosFromMyChannel(&knownVideos)		// send by reference because we will change it

//	tomlPrintKnownVids(knownVideos)

	fillInDurations(&knownVideos)

	saveLocalKnownVideos(knownVideos)

	// for _, video := range playlist {
	// 	// sometimes Snippets are nil but I am not sure why
	// 	if video.Snippet != nil {
	// 		videoId := video.Snippet.ResourceId.VideoId
	// 		title := video.Snippet.Title
	// 		match, _ := regexp.MatchString("Live Stream:", title)
	// 		if match {
	// 			fmt.Printf("%v  \"%v\" %v \r\n", videoId, title, match)
	// 		} else {
	// 			fmt.Printf("%v skipped\r\n", title)
	// 		}
	// 	}
	// }
}
