package server

import (
	"sync"
	"time"
)

// refusalBurst is how many tailnet refusal lines the log takes in a
// minute, from every caller together.
const refusalBurst = 30

// refusalLog bounds the lines the daemon writes about refused tailnet
// requests (davison/md-notes#240). A browser holding a dead cookie makes a
// request, and so a refusal, for every asset and every API call on a page,
// and a hostile node on the tailnet can make as many as it likes; neither
// should be able to fill the log. So the same reason from the same caller
// is written once a minute, and every caller together gets refusalBurst
// lines a minute. The caller is named by X-Forwarded-For, which the caller
// can vary — the same weakness the login throttle's key has — and the
// total is what bounds that. The first line after a minute with lines
// left out says how many, so a flood shows as a flood rather than as
// silence.
type refusalLog struct {
	mu         sync.Mutex
	window     time.Time
	written    int
	suppressed int
	seen       map[string]bool
}

// allow reports whether a refusal from key may be written at now, and how
// many were left out in the last capped minute and when that minute began,
// which the caller reports once. The count is written with the next
// refusal, whenever that comes — there is no timer — so the line names the
// minute it is about rather than calling it the last one.
func (l *refusalLog) allow(key string, now time.Time) (ok bool, leftOut int, from time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil || !now.Before(l.window.Add(time.Minute)) {
		leftOut, from = l.suppressed, l.window
		l.window, l.written, l.suppressed, l.seen = now, 0, 0, map[string]bool{}
	}
	if l.seen[key] {
		return false, leftOut, from
	}
	if l.written >= refusalBurst {
		l.suppressed++
		return false, leftOut, from
	}
	l.seen[key] = true
	l.written++
	return true, leftOut, from
}
