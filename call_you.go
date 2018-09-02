package main

import(
	"google.golang.org/api/youtube/v3"
)

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
// This does not reliably returns the items sorted by published date.  (it is close, but not perfect)
// If they were returned in sorted order, I could skip calling next page when I started getting hits on knownVideos
// Incorrect sort might be related to https://issuetracker.google.com/issues/35176658
func playlistItemsList(service *youtube.Service, part string, playlistId string, pageToken string, numItems int64) *youtube.PlaylistItemListResponse {
	call := service.PlaylistItems.List(part)
	call = call.MaxResults(numItems)			// Hopefully speed things overall by requiring fewer calls  (default 5, max 50)
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
