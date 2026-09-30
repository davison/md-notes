package server

import "strings"

// browserFamily names the browser and platform a User-Agent claims, in the
// coarse terms a person reading the daemon's log would use — "Chrome on
// Android" — so a login can be told apart by device without the log
// holding the User-Agent itself (davison/md-notes#240). It is a label for
// a human, never a decision: a User-Agent is whatever the caller wrote.
func browserFamily(ua string) string {
	browser := "unknown browser"
	// Order matters: most browsers claim to be Chrome and Safari as well.
	for _, b := range []struct{ marker, name string }{
		{"Edg", "Edge"},
		{"SamsungBrowser/", "Samsung Internet"},
		{"OPR/", "Opera"},
		{"Firefox/", "Firefox"},
		{"FxiOS/", "Firefox"},
		{"CriOS/", "Chrome"},
		{"; wv)", "Android WebView"},
		{"Chromium/", "Chromium"},
		{"Chrome/", "Chrome"},
		{"Safari/", "Safari"},
	} {
		if strings.Contains(ua, b.marker) {
			browser = b.name
			break
		}
	}
	if browser == "unknown browser" {
		return browser
	}
	for _, p := range []struct{ marker, name string }{
		{"Android", "Android"},
		{"iPhone", "iPhone"},
		{"iPad", "iPad"},
		{"CrOS", "ChromeOS"},
		{"Windows", "Windows"},
		{"Macintosh", "macOS"},
		{"Linux", "Linux"},
	} {
		if strings.Contains(ua, p.marker) {
			return browser + " on " + p.name
		}
	}
	return browser
}
