package diagram

import (
	"context"
	"errors"
	"fmt"
)

var errTimeout = errors.New("render deadline")

// Render draws the mermaid flowchart in src as an SVG document in theme,
// under DefaultLimits.
//
// The error is a *Refusal when the block is outside the supported subset,
// does not parse, or hits a bound — including the render deadline — and
// the caller should then show the block as code. Any other error is the
// caller's own context ending, or a theme that is not valid.
//
// Render is safe for concurrent use.
func Render(ctx context.Context, src []byte, theme Theme) ([]byte, error) {
	return RenderLimits(ctx, src, theme, DefaultLimits)
}

// RenderLimits is Render with bounds of the caller's choosing.
func RenderLimits(ctx context.Context, src []byte, theme Theme, lim Limits) (out []byte, err error) {
	if err := theme.valid(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeoutCause(ctx, lim.Timeout, errTimeout)
	defer cancel()
	defer func() {
		// The package is written not to panic on any input, and the fuzz
		// tests hold it to that; this is the second line, so that a bug
		// costs one block its diagram rather than the daemon a goroutine.
		if r := recover(); r != nil {
			out, err = nil, refuse(Unsupported, 0, "internal error drawing the diagram: %v", r)
		}
	}()
	f, err := Parse(src, lim)
	if err != nil {
		return nil, err
	}
	d, err := layout(ctx, f, lim)
	if err != nil {
		if errors.Is(context.Cause(ctx), errTimeout) {
			return nil, refuse(Limit, 0, "the layout took longer than %v", lim.Timeout)
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(context.Cause(ctx), errTimeout) {
			return nil, refuse(Limit, 0, "the layout took longer than %v", lim.Timeout)
		}
		return nil, fmt.Errorf("diagram: %w", err)
	}
	return writeSVG(d, theme), nil
}
