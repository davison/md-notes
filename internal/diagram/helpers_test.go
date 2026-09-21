package diagram

import (
	"strconv"
	"strings"
)

func itoa(i int) string { return strconv.Itoa(i) }

// randomSource is a flowchart of n labelled nodes and e edges between
// pseudo-random pairs: dense, crossing and cyclic, the costly kind of
// input a hand-written diagram rarely is.
func randomSource(n, e int, seed uint64) []byte {
	r := seed
	next := func(m int) int {
		r = r*6364136223846793005 + 1442695040888963407
		return int((r >> 33) % uint64(m))
	}
	var b strings.Builder
	b.WriteString("flowchart TB\n")
	for i := 0; i < n; i++ {
		b.WriteString("n" + itoa(i) + "[node number " + itoa(i) + "]\n")
	}
	for i := 0; i < e; i++ {
		a, c := next(n), next(n)
		if a == c {
			c = (c + 1) % n
		}
		b.WriteString("n" + itoa(a) + " --> n" + itoa(c) + "\n")
	}
	return []byte(b.String())
}
