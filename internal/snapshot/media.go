package snapshot

// The titles every adapter that reads a media server gives its parts, so
// that the page adds up every server's plays, Jellyfin's and Plex's alike,
// and names each one's section by its source beside another's.
var (
	PlaysFigure  = Text{Key: "figure.plays"}
	MediaSection = Text{Key: "section.media"}
)

// PlayingNow is the key of the plays figure's detail, what plays now, as
// {n}.
const PlayingNow = "figure.playing_now"
