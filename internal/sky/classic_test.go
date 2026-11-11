package sky

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// board draws a life board from rows of '#' and '.'.
func board(rows ...string) ([]bool, int, int) {
	w, h := len(rows[0]), len(rows)
	cells := make([]bool, w*h)
	for y, row := range rows {
		for x, c := range row {
			cells[y*w+x] = c == '#'
		}
	}
	return cells, w, h
}

func TestAGliderMovesOneCellDiagonallyEveryFourGenerations(t *testing.T) {
	cells, w, h := board(
		".#......",
		"..#.....",
		"###.....",
		"........",
		"........",
		"........",
	)
	want, _, _ := board(
		"........",
		"..#.....",
		"...#....",
		".###....",
		"........",
		"........",
	)
	for range 4 {
		cells = lifeStep(cells, w, h)
	}
	if !slices.Equal(cells, want) {
		t.Error("a glider did not move one cell down and right in four generations")
	}
}

func TestABlinkerHasPeriodTwo(t *testing.T) {
	cells, w, h := board(".....", "..#..", "..#..", "..#..", ".....")
	once := lifeStep(cells, w, h)
	if slices.Equal(once, cells) {
		t.Fatal("a blinker stood still")
	}
	if !slices.Equal(lifeStep(once, w, h), cells) {
		t.Error("a blinker did not return after two generations")
	}
}

// rows runs an elementary rule from a single cell for n rows and draws them.
func rows(rule, width, n int) []string {
	row := make([]bool, width)
	row[width/2] = true
	var out []string
	for range n {
		var b strings.Builder
		for _, on := range row {
			if on {
				b.WriteByte('#')
			} else {
				b.WriteByte('.')
			}
		}
		out = append(out, b.String())
		row = ruleStep(row, rule)
	}
	return out
}

func TestRule30And90GrowAsPublished(t *testing.T) {
	for rule, want := range map[int][]string{
		30: {"....#....", "...###...", "..##..#..", ".##.####."},
		90: {"....#....", "...#.#...", "..#...#..", ".#.#.#.#."},
	} {
		if got := rows(rule, 9, 4); !slices.Equal(got, want) {
			t.Errorf("rule %d grew\n%s\nwant\n%s", rule, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

func TestLangtonsAntTakesItsFirstSteps(t *testing.T) {
	const w, h = 5, 5
	black := make([]bool, w*h)
	ant := [3]int{2, 2, 0}
	// On white cells it turns right each time, so its first four steps go
	// round a square and leave all four cells black.
	want := [][2]int{{3, 2}, {3, 3}, {2, 3}, {2, 2}}
	for i, at := range want {
		ant = antStep(black, w, h, ant)
		if ant[0] != at[0] || ant[1] != at[1] {
			t.Fatalf("step %d took the ant to %d,%d, want %d,%d", i+1, ant[0], ant[1], at[0], at[1])
		}
	}
	for _, at := range [][2]int{{2, 2}, {3, 2}, {3, 3}, {2, 3}} {
		if !black[at[1]*w+at[0]] {
			t.Errorf("cell %d,%d was left white", at[0], at[1])
		}
	}
	// Now on a black cell it turns left.
	ant = antStep(black, w, h, ant)
	if ant[0] != 1 || ant[1] != 2 {
		t.Errorf("on a black cell the ant went to %d,%d, want 1,2", ant[0], ant[1])
	}
}

func TestTheJuliaSetOfZeroIsTheUnitDisc(t *testing.T) {
	if juliaDepth(0.5, 0.5, 0, 0, 32) != 1 {
		t.Error("a point inside the unit circle escaped z² from zero")
	}
	if juliaDepth(1.5, 0, 0, 0, 32) == 1 {
		t.Error("a point outside the unit circle never escaped z² from zero")
	}
}

func TestTheMazeIsASpanningTree(t *testing.T) {
	const cw, ch = 9, 7
	gw, gh := 2*cw+1, 2*ch+1
	open := make([]bool, gw*gh)
	seen := make([]bool, cw*ch)
	seen[0] = true
	open[gw+1] = true
	stack := [][2]int{{0, 0}}
	rng := rand.New(rand.NewPCG(1, 2))
	carved := 0
	for len(stack) > 0 {
		stack = mazeStep(stack, seen, cw, ch, rng, func(x, y int) {
			open[y*gw+x] = true
			carved++
		})
	}
	// A spanning tree of n cells has n-1 passages: every cell after the first
	// is opened once, with the wall between it and the cell it came from.
	if carved != 2*(cw*ch-1) {
		t.Fatalf("the maze opened %d squares, want %d", carved, 2*(cw*ch-1))
	}
	reached := map[[2]int]bool{}
	var walk func(x, y int)
	walk = func(x, y int) {
		if x < 0 || y < 0 || x >= gw || y >= gh || !open[y*gw+x] || reached[[2]int{x, y}] {
			return
		}
		reached[[2]int{x, y}] = true
		walk(x+1, y)
		walk(x-1, y)
		walk(x, y+1)
		walk(x, y-1)
	}
	walk(1, 1)
	for y := range ch {
		for x := range cw {
			if !reached[[2]int{2*x + 1, 2*y + 1}] {
				t.Fatalf("cell %d,%d cannot be reached", x, y)
			}
		}
	}
}

func TestABubbleSortFinishesSorted(t *testing.T) {
	bars := []float64{5, 1, 4, 2, 8, 0, 3}
	i, j := 0, 0
	for steps := 0; bubbleStep(bars, &i, &j); steps++ {
		if steps > 100 {
			t.Fatal("the sort never finished")
		}
	}
	if !slices.IsSorted(bars) {
		t.Errorf("the sort finished with %v", bars)
	}
}
