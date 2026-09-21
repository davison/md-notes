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

// slowest is the slowest block inside every default bound that a search
// of 300 random graphs (40 to 200 nodes, n to 2n edges) turned up: about
// 210ms to lay out on the machine #170 was measured on. The deadline tests
// use it so that a layout that ignored its deadline would visibly overrun.
func slowest() []byte { return randomSource(189, 260, 177) }
