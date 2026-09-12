package events

import (
	"net/url"
	"strings"
)

// Native organizers have an actual public MyVshage profile. Keep the
// environment's host; do not link a DEV organizer to an unrelated PROD slug.
func organizerPageURL(slug *string) *string {
	if slug == nil || strings.TrimSpace(*slug) == "" {
		return nil
	}
	base, err := url.Parse(publicBaseURL)
	if err != nil || !strings.HasPrefix(base.Hostname(), "afisha.") {
		return nil
	}
	base.Host = strings.Replace(base.Host, "afisha.", "my.", 1)
	base.Path = "/p/" + *slug
	base.RawPath = "/p/" + url.PathEscape(*slug)
	base.RawQuery = ""
	base.Fragment = ""
	s := base.String()
	return &s
}
