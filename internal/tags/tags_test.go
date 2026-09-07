package tags

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExtract(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{"frontmatter list", "---\ntags: [Go, notes/daily, '#x']\n---\nbody\n", []string{"go", "notes/daily", "x"}},
		{"frontmatter string", "---\ntags: a, b  c\n---\n", []string{"a", "b", "c"}},
		{"inline start and mid", "#alpha at start\nthen #beta-2 and (#gamma) here\n", []string{"alpha", "beta-2", "gamma"}},
		{"heading is not a tag", "# Heading\n## Sub\n", nil},
		{"numeric is not a tag", "see #123 and #2024\n", nil},
		{"url fragment is not a tag", "https://x.example/page#section text\n", nil},
		{"link target is not a tag", "see [anchor](#anchor-link) and [x](../a.md#frag) then #real\n", []string{"real"}},
		{"capitalised key", "---\nTags: [Up]\n---\n", []string{"up"}},
		{"exact key wins over other spellings", "---\nTAGS: [shouty]\ntags: [exact]\n---\n", []string{"exact"}},
		{"reference definition is not a tag", "[ref]: #ref-anchor\n[r2]: https://x#f 'title'\n#real\n", []string{"real"}},
		{"fenced code excluded", "```\n#notatag\n```\n#real\n", []string{"real"}},
		{"tilde fence excluded", "~~~\n#notatag\n~~~\n", nil},
		{"inline code excluded", "use `#notatag` but #yes\n", []string{"yes"}},
		{"case folded and deduplicated", "#Tag #tag #TAG\n", []string{"tag"}},
		{"unicode", "#café #日本語\n", []string{"café", "日本語"}},
		{"both sources", "---\ntags: [one]\n---\n#two\n", []string{"one", "two"}},
		{"malformed frontmatter is body", "---\nnot: [\n---\n#body\n", []string{"body"}},
		{"trailing punctuation trimmed", "#end/ #dash-\n", []string{"dash", "end"}},
		{"empty", "", nil},
	}
	for _, c := range cases {
		got := Extract([]byte(c.src))
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Extract = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCollect(t *testing.T) {
	root := t.TempDir()
	write := func(p, s string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755)
		os.WriteFile(filepath.Join(root, p), []byte(s), 0o644)
	}
	write("a.md", "---\ntags: [shared, only-a]\n---\n")
	write("b/b.md", "#shared and #only-b\n")
	write("c.md", "#shared\n")
	got := Collect(context.Background(), root, []string{"a.md", "b/b.md", "c.md", "missing.md"})
	want := []Tag{
		{Name: "shared", Count: 3, Notes: []string{"a.md", "b/b.md", "c.md"}},
		{Name: "only-a", Count: 1, Notes: []string{"a.md"}},
		{Name: "only-b", Count: 1, Notes: []string{"b/b.md"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Collect = %+v, want %+v", got, want)
	}
}

func TestCollectCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.md"), []byte("#x"), 0o644)
	if got := Collect(ctx, root, []string{"a.md"}); len(got) != 0 {
		t.Fatalf("cancelled Collect = %v", got)
	}
}
