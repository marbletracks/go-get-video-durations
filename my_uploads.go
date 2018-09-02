package main

import (
	"fmt"
	"log"
	"time"
	"strings"	// needed to create a string of video IDs, separated by commas
	"regexp"	// will be needed to parse Titles when searching for "Live Stream:"
	"bytes"		// for debugging Encoder
	"os"		// for Encoder

	"google.golang.org/api/youtube/v3"
	"github.com/BurntSushi/toml"
)

const localPathToKnownVideosFile = "/Users/thunderrabbit/mt3.com/data/playlists/knownvideos.toml"

type MT3VideoType uint8
// Hugo will do different things with different types of videos
const (
	Unknown MT3VideoType = iota
	Livestream
	Snippet
)

// This is the structure to be used in localPathToKnownVideosFile
type tomlKnownVideos struct {
	Videos map[string]videoMeta
}

// Each video will have basic data.
// Duration will allow me to report just how long I have spent on Marble Track 3
type videoMeta struct {
  VideoId string
  Title string
  Published time.Time // requires `import time`
  Duration time.Duration
  VideoType MT3VideoType
}


// this needs to return something, basically an enum
func determineVideoTypeBasedOnTitle(title string) MT3VideoType {
	match, _ := regexp.MatchString("Live Stream", title)
	if match {
		return Livestream
	} else {
		return Snippet
	}
	return Unknown
}

// knownVideos is the list of videos in our local TOML file
// playlistItem is one of the myriad videos in my channel
// This looks at each video ID to see if we need to add it to knownVideos
func addNewVideosToList(playlistItem *youtube.PlaylistItem, knownVideos *tomlKnownVideos) bool {
	foundNewVideos := false
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
		foundNewVideos = true
		knownVideos.Videos[playlistItem.Snippet.ResourceId.VideoId] =
			videoMeta{
				VideoId:playlistItem.Snippet.ResourceId.VideoId,
				Title:playlistItem.Snippet.Title,
				Published:vidPublishTime,
				Duration:vidDuration,
				VideoType:determineVideoTypeBasedOnTitle(playlistItem.Snippet.Title),
			}
	}
	return foundNewVideos
}

// Download from Youtube all the videos in my channel
// so we can look for new ones that do not exist in local TOML file
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
		fmt.Printf("Checking for new videos in list %s\r\n", playlistId)

		nextPageToken := ""
		var numItemsPerPage int64 = 5			// max 50 https://developers.google.com/youtube/v3/docs/playlistItems/list#parameters
		foundNewVideos := false					// if we added videos, we will look for next page of videos
		for {
			// Retrieve next set of items in the playlist.
			// Items are not returned in perfectly sorted order, so just go through all pages to get all items
			// Revisit this if it gets too slow
			playlistResponse := playlistItemsList(service, "snippet,ContentDetails", playlistId, nextPageToken, numItemsPerPage)
			
			for _, playlistItem := range playlistResponse.Items {
				foundNewVideos = addNewVideosToList(playlistItem, knownVideos)
			}

			if foundNewVideos {
				fmt.Println("Found some new videos.  Let's look for more!")
			} else {
				fmt.Printf("Searched %v videos and found nothing new.  Let's move on.\r\n",numItemsPerPage)
				// The results are not exactly ordered by publishDate, so there could be cases where we didn't find expected videos
				fmt.Println("If we should have found some, increase numItemsPerPage or remove \"!foundNewVideos ||\" from code")
			}
			// Set the token to retrieve the next page of results
			// or exit the loop if all results have (apparently) been retrieved.
			nextPageToken = playlistResponse.NextPageToken
			if !foundNewVideos || nextPageToken == "" {
				break
			}
		}
	}
}

// for debugging, but not currently used
func tomlPrintKnownVids(knownVideos tomlKnownVideos) {
	buf := new(bytes.Buffer)
	err := toml.NewEncoder(buf).Encode(knownVideos)
	check(err)
	fmt.Println(buf.String())
}

// a TOML list of videos is stored locally to reduce the number of times we have to contact Youtube API
// This loads the file and returns as a struct of type tomlKnownVideos
func loadLocalKnownVideos() tomlKnownVideos {
	var knownVideos tomlKnownVideos			// knownVideos will be read from local TOML file

	_, err := toml.DecodeFile(localPathToKnownVideosFile, &knownVideos)
	if(err != nil) {
		fmt.Println("Error while loading knownVideos.TOML:")
		fmt.Println(localPathToKnownVideosFile)
		fmt.Println("Should remove '!foundNewVideos ||' and increase numItemsPerPage to 50 then rerun until rebuilt")
		var emptyKnownVideos tomlKnownVideos
		return emptyKnownVideos
	}

	return knownVideos
}


// a TOML list of videos is stored locally to reduce the number of times we have to contact Youtube API
// This saves the file
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
// The IDs will be sent to YouTube API to get the video Durations
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

	for _, item := range response.Items {
		// Google returns a format like PT1H45M41S for the Duration
		// For time.ParseDuration, we have to:
		//    crop off the PT using [2:]
		//    change to lower case
		// to send something like this:  1h45m41s
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

	loadNewVideosFromMyChannel(&knownVideos)		// send by reference because we will add new videos from Youtube

	fillInDurations(&knownVideos)					// send by reference so we can update the Durations

	saveLocalKnownVideos(knownVideos)
}
