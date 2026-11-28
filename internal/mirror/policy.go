package mirror

import "strings"

// Policy is what offline copies keep while they are on.
type Policy struct {
	Limits Limits
	// Images keeps the images a page loads; WarmImages also fetches them ahead of time.
	Images     bool
	WarmImages bool
	Video      bool
	Audio      bool
}

// DefaultPolicy is what a new install keeps.
var DefaultPolicy = PolicyFor("default", "500mb", false, true, true, true, 0)

// PolicyFor maps the offline settings to a Policy. Default keeps everything but video
// and audio inside its space choice and fetches no images ahead of time. Fully offline
// keeps everything with no space or age limit, or, with limits on, the kinds switched
// on inside limitMB (0 is no limit).
func PolicyFor(mode, space string, limited, images, video, audio bool, limitMB int) Policy {
	if mode != "full" {
		return Policy{Limits: LimitsFor(space), Images: true}
	}
	if !limited {
		return Policy{Images: true, WarmImages: true, Video: true, Audio: true}
	}
	return Policy{Limits: Limits{Total: int64(limitMB) << 20}, Images: images, WarmImages: images, Video: video, Audio: audio}
}

// AllowsType says whether a copy with this Content-Type may be kept.
func (p Policy) AllowsType(contentType string) bool {
	t := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.HasPrefix(t, "image/"):
		return p.Images
	case strings.HasPrefix(t, "video/"):
		return p.Video
	case strings.HasPrefix(t, "audio/"):
		return p.Audio
	}
	return true
}

// AllowsDest says whether a request destination (or Resource Timing kind) may be kept.
func (p Policy) AllowsDest(dest string) bool {
	switch dest {
	case "image", "img":
		return p.Images
	case "video":
		return p.Video
	case "audio":
		return p.Audio
	case "track":
		return false
	}
	return true
}
