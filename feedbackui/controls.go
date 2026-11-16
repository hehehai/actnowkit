package feedbackui

// The window's controls, drawn as the design has them unless the app's
// Theme draws its own; and the small glyphs (no font has them all).

import (
	"github.com/egoist/mygo/ui"
)

// frame is what a frame of the view draws with.
type frame struct {
	c  *ui.Context
	w  *Window
	p  *Palette
	th *Theme
	fs float32 // body font size
	r  float32 // control radius
}

func (f *frame) text(s string) ui.Element {
	return ui.Text(f.c, s).FontSize(f.fs).TextColor(f.p.Text)
}

func (f *frame) muted(s string) ui.Element {
	return ui.Text(f.c, s).FontSize(f.fs - 1.5).TextColor(f.p.TextMuted)
}

func (f *frame) label(s string) ui.Element {
	return ui.Text(f.c, s).FontSize(f.fs).TextColor(f.p.TextSecondary)
}

// button is a secondary button.
func (f *frame) button(key, label string) ui.Element {
	c := f.c.Key(key)
	if f.th.Button != nil {
		return f.th.Button(c, label)
	}
	b := ui.ButtonBase(c).Padding(6, 14).Radius(f.r).Background(f.p.Background).Border(1, f.p.Border).Label(label).Shrink(0)
	b.Children(func() {
		ui.Text(f.c, label).FontSize(f.fs).FontWeight(500).TextColor(f.p.Text).NoWrap()
	})
	return b
}

// primary is the primary button.
func (f *frame) primary(key, label string, disabled bool) ui.Element {
	c := f.c.Key(key)
	if f.th.PrimaryButton != nil {
		return f.th.PrimaryButton(c, label).Disabled(disabled)
	}
	b := ui.ButtonBase(c).Padding(6, 14).Radius(f.r).Background(f.p.PrimaryFill).Border(1, f.p.PrimaryFill).Label(label).Shrink(0).Disabled(disabled)
	if disabled {
		b.Opacity(0.45)
	}
	b.Children(func() {
		ui.Text(f.c, label).FontSize(f.fs).FontWeight(500).TextColor(f.p.PrimaryText).NoWrap()
	})
	return b
}

// link is a text button in the accent color.
func (f *frame) link(key, label string, size float32) ui.Element {
	b := ui.ButtonBase(f.c.Key(key)).Padding(0).Role(ui.RoleLink).Label(label).Justify(ui.Start).Shrink(0)
	b.Children(func() {
		ui.Text(f.c, label).FontSize(size).TextColor(f.p.Accent)
	})
	return b
}

// segmented is the type's control: a track with the chosen segment raised.
func (f *frame) segmented(key string, sel *int, labels ...string) ui.Element {
	if f.th.Segmented != nil {
		return f.th.Segmented(f.c.Key(key), sel, labels...)
	}
	row := ui.Row(f.c.Key(key)).Background(f.p.Track).Radius(f.r + 1).Padding(2).Gap(2).Shrink(0).Role(ui.RoleRadioGroup)
	row.Children(func() {
		for i, l := range labels {
			on := i == *sel
			b := ui.ButtonBase(f.c.Key(i)).Padding(4, 12).Radius(f.r - 1).Role(ui.RoleRadio).Checked(on).Label(l)
			if on {
				b.Background(f.p.SegmentOn).Shadow(0, 1, 2, 0, f.p.Shadow)
			}
			b.Children(func() {
				t := ui.Text(f.c, l).FontSize(f.fs).NoWrap()
				if on {
					t.TextColor(f.p.Text).FontWeight(500)
				} else {
					t.TextColor(f.p.TextSecondary)
				}
			})
			if b.Clicked() {
				*sel = i
			}
		}
	})
	return row
}

// toggle is a switch.
func (f *frame) toggle(key string, on *bool, label string) ui.Element {
	if f.th.Switch != nil {
		return f.th.Switch(f.c.Key(key), on).Label(label)
	}
	return ui.Switch(f.c.Key(key), on).Label(label).Shrink(0)
}

// ---- glyphs ----

// stroke draws a polyline through pts (x, y pairs) with round caps.
func stroke(p *ui.Painter, width float32, col ui.Color, pts ...float32) {
	var path ui.Path
	path.MoveTo(pts[0], pts[1])
	for i := 2; i+1 < len(pts); i += 2 {
		path.LineTo(pts[i], pts[i+1])
	}
	p.StrokePath(&path, width, col)
}

// cross is an ✕ of side s.
func cross(c *ui.Context, s, width float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(s, s).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		in := s * 0.22
		stroke(p, width, col, r.X+in, r.Y+in, r.X+s-in, r.Y+s-in)
		stroke(p, width, col, r.X+s-in, r.Y+in, r.X+in, r.Y+s-in)
	})
}

// plus is a + of side s.
func plus(c *ui.Context, s, width float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(s, s).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Line(r.X+s/2, r.Y+1, r.X+s/2, r.Y+s-1, width, col)
		p.Line(r.X+1, r.Y+s/2, r.X+s-1, r.Y+s/2, width, col)
	})
}

// check is a ✓ of side s.
func check(c *ui.Context, s, width float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(s, s).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		x, y := r.X, r.Y
		stroke(p, width, col, x+s*0.18, y+s*0.52, x+s*0.42, y+s*0.76, x+s*0.84, y+s*0.26)
	})
}

// chevron points left (dir -1) or right (1).
func chevron(c *ui.Context, s, width float32, dir float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(s, s).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+s/2, r.Y+s/2
		d := s * 0.2
		stroke(p, width, col, cx-d*dir, cy-2*d, cx+d*dir, cy, cx-d*dir, cy+2*d)
	})
}

// playButton is the white disc with a triangle over a video's tile.
func playButton(c *ui.Context, s float32) ui.Element {
	return ui.Box(c).Size(s, s).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, ui.RGBA(255, 255, 255, 0.88), s/2)
		var tri ui.Path
		cx, cy := r.X+s/2+1.5, r.Y+s/2
		tri.MoveTo(cx-4.5, cy-6).LineTo(cx+5, cy).LineTo(cx-4.5, cy+6).Close()
		p.FillPath(&tri, ui.Hex("#111111"))
	})
}

// dot is a filled circle.
func dot(c *ui.Context, s float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(s, s).Radius(s / 2).Background(col).Shrink(0)
}
