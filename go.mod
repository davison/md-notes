module github.com/davison/md-notes

go 1.27

require (
	// Held at v2.2.0 (current is v2.27.0). chroma's `github` style, which
	// internal/render/gencss tones both colour schemes from, no longer
	// styles GenericHeading, GenericStrong, GenericSubheading,
	// GenericPrompt, GenericError, GenericTraceback or NameException, so
	// the upgrade takes the colour off the prompt in a console block, the
	// hunk header in a diff and bold in a markdown block. Measured on
	// davison/md-notes#147; the hold-back and what would lift it are
	// recorded there.
	github.com/alecthomas/chroma/v2 v2.2.0
	github.com/fsnotify/fsnotify v1.10.1
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/yuin/goldmark v1.8.6
	github.com/yuin/goldmark-highlighting/v2 v2.0.0-20230729083705-37449abec8cc
	golang.org/x/net v0.59.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/dlclark/regexp2 v1.7.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	golang.org/x/sys v0.48.0 // indirect
)
