package activity

// The stack's geometry (the family's window spec §2.2), in DIPs at 100 %:
// closed, the first card in full and the next two peeking out under it,
// each a little smaller; open, a list of Show cards with gaps.

// Measures of the stack.
const (
	CardHeight = 44 // a card
	CardGap    = 6  // between cards laid out (open)
	CardPeek   = 5  // how much of each card behind the first shows (closed)
	Behind     = 2  // how many cards peek out under the first (closed)
	Show       = 4  // how many cards the open stack lists ("View all" the rest)
	// ShrinkStep is how much smaller each card behind is (closed).
	ShrinkStep = 0.06
)

// Shown is how many of n cards the stack draws.
func Shown(n int) int { return min(n, Show) }

// Height is the stack's height for n cards (the line under it apart).
func Height(n int, open bool) float32 {
	if n <= 0 {
		return 0
	}
	if open && n > 1 {
		s := Shown(n)
		return float32(s)*CardHeight + float32(s-1)*CardGap
	}
	return CardHeight + float32(min(n-1, Behind))*CardPeek
}

// Pose is where card i sits: Top its place in the open list; closed, Y
// moves it from there (up under the first) and Scale shrinks it, Fade
// hides it (beyond Behind). The first card's pose is the identity.
type Pose struct {
	Top, Y, Scale float32
	Fade          bool
}

// Poses are the poses of the cards the stack draws (Shown(n)), the first
// first; draw them last to first, so that the first is on top.
func Poses(n int, open bool) []Pose {
	s := Shown(n)
	out := make([]Pose, s)
	for i := range out {
		top := float32(i) * (CardHeight + CardGap)
		p := Pose{Top: top, Scale: 1}
		if !open && i > 0 {
			p.Y = float32(min(i, Behind))*CardPeek - top
			p.Scale = 1 - ShrinkStep*float32(min(i, Behind+1))
			p.Fade = i > Behind
		}
		out[i] = p
	}
	return out
}
