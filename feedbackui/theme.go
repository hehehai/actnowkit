package feedbackui

import "github.com/egoist/mygo/ui"

// Palette is the colors of the window, light or dark. The names follow
// the grey scale of the design (g0 the window, g9 the text).
type Palette struct {
	Background    ui.Color // g0: the window, cards, the chosen segment's face (light)
	Raised        ui.Color // g1: text fields, the footer
	Track         ui.Color // g2: the segmented control's track, tiles loading, the number's chip
	Separator     ui.Color // g3: lines between the rows
	Border        ui.Color // g4: fields, tiles, buttons
	BorderStrong  ui.Color // g5: the dashed "Add" tile
	Placeholder   ui.Color // g6
	TextMuted     ui.Color // g7: hints, captions
	TextSecondary ui.Color // g8: labels
	Text          ui.Color // g9
	SegmentOn     ui.Color // the chosen segment's face
	Accent        ui.Color // links, switches, check boxes, the progress
	AccentText    ui.Color // text on Accent
	Danger        ui.Color
	DangerBG      ui.Color // the error report's card
	PrimaryFill   ui.Color // the primary button (g9)
	PrimaryText   ui.Color // its text (g0)
	Shadow        ui.Color
}

// LightPalette is Dicta's light colors, the default.
func LightPalette() *Palette {
	return &Palette{
		Background: ui.Hex("#ffffff"), Raised: ui.Hex("#f7f7f7"), Track: ui.Hex("#f2f2f2"), Separator: ui.Hex("#ebebeb"),
		Border: ui.Hex("#e0e0e0"), BorderStrong: ui.Hex("#c7c7c7"), Placeholder: ui.Hex("#a3a3a3"), TextMuted: ui.Hex("#6e6e6e"),
		TextSecondary: ui.Hex("#454545"), Text: ui.Hex("#111111"), SegmentOn: ui.Hex("#ffffff"),
		Accent: ui.Hex("#0d9384"), AccentText: ui.Hex("#ffffff"), Danger: ui.Hex("#d93a33"), DangerBG: ui.Hex("#fdf0ef"),
		PrimaryFill: ui.Hex("#111111"), PrimaryText: ui.Hex("#ffffff"), Shadow: ui.RGBA(0, 0, 0, 0.12),
	}
}

// DarkPalette is Dicta's dark colors, the default.
func DarkPalette() *Palette {
	return &Palette{
		Background: ui.Hex("#151515"), Raised: ui.Hex("#1c1c1c"), Track: ui.Hex("#232323"), Separator: ui.Hex("#2b2b2b"),
		Border: ui.Hex("#353535"), BorderStrong: ui.Hex("#4a4a4a"), Placeholder: ui.Hex("#6b6b6b"), TextMuted: ui.Hex("#a3a3a3"),
		TextSecondary: ui.Hex("#d0d0d0"), Text: ui.Hex("#f2f2f2"), SegmentOn: ui.Hex("#353535"),
		Accent: ui.Hex("#2bb3a3"), AccentText: ui.Hex("#06201c"), Danger: ui.Hex("#ff6b61"), DangerBG: ui.Hex("#33201f"),
		PrimaryFill: ui.Hex("#f2f2f2"), PrimaryText: ui.Hex("#151515"), Shadow: ui.RGBA(0, 0, 0, 0.4),
	}
}

// PaletteOf derives a palette from a MyGo theme (an app whose own theme
// is set on its windows: Theme.FromMyGo).
func PaletteOf(t *ui.Theme) *Palette {
	p := LightPalette()
	if t.Dark {
		p = DarkPalette()
	}
	p.Background, p.Raised, p.Track = t.Background, t.Surface, t.SurfaceHover
	p.Border, p.BorderStrong = t.Border, t.Border
	p.Text, p.TextSecondary, p.TextMuted, p.Placeholder = t.Text, t.Text, t.TextMuted, t.TextMuted
	p.Accent, p.AccentText, p.Danger = t.Accent, t.AccentText, t.Danger
	p.PrimaryFill, p.PrimaryText = t.Text, t.Background
	p.SegmentOn = t.Background
	if t.Dark {
		p.SegmentOn = t.SurfacePressed
	}
	return p
}

// Theme is how the window looks. The zero Theme is Dicta's look (the
// design's), light or dark as the system is.
type Theme struct {
	// Light and Dark replace the default palettes (nil: LightPalette,
	// DarkPalette).
	Light, Dark *Palette
	// FromMyGo takes the colors from the MyGo theme of the frame
	// (c.Theme(): the system's, or the one the app sets) instead.
	FromMyGo bool
	// DarkMode overrides the system's appearance: the app's own setting
	// (nil, or a func answering nil: the system's).
	DarkMode func() *bool
	// Radius of the buttons and fields (0: 7); the cards and tiles are 3
	// more.
	Radius float32
	// Font and MonoFont families ("": the system's; MonoFont "monospace"),
	// FontSize of the body text (0: 13).
	Font, MonoFont string
	FontSize       float32

	// Optional drawing of the controls, for an app whose own differ (nil:
	// the window's). They return the element; the window asks Clicked,
	// sets Disabled and reads the value.
	PrimaryButton func(c *ui.Context, label string) ui.Element
	Button        func(c *ui.Context, label string) ui.Element
	Segmented     func(c *ui.Context, selected *int, labels ...string) ui.Element
	Switch        func(c *ui.Context, on *bool) ui.Element
}

// dark: the app's appearance, else the frame's (the system's).
func (th *Theme) dark(t *ui.Theme) bool {
	if th.DarkMode != nil {
		if d := th.DarkMode(); d != nil {
			return *d
		}
	}
	return t.Dark
}

// palette for a frame whose MyGo theme is t.
func (th *Theme) palette(t *ui.Theme) *Palette {
	if d := th.dark(t); d != t.Dark {
		base := ui.LightTheme()
		if d {
			base = ui.DarkTheme()
		}
		base.FontSize, base.Font = t.FontSize, t.Font
		t = base
	}
	switch {
	case th.FromMyGo:
		return PaletteOf(t)
	case t.Dark && th.Dark != nil:
		return th.Dark
	case t.Dark:
		return darkDefault
	case th.Light != nil:
		return th.Light
	}
	return lightDefault
}

var lightDefault, darkDefault = LightPalette(), DarkPalette()

func (th *Theme) radius() float32 {
	if th.Radius > 0 {
		return th.Radius
	}
	return 7
}

func (th *Theme) fontSize() float32 {
	if th.FontSize > 0 {
		return th.FontSize
	}
	return 13
}

func (th *Theme) mono() string {
	if th.MonoFont != "" {
		return th.MonoFont
	}
	return "monospace"
}

// mygoTheme is the frame's MyGo theme in the palette's colors, for the
// widgets the window takes from MyGo (text fields, switches, check boxes).
func (th *Theme) mygoTheme(base *ui.Theme, p *Palette) *ui.Theme {
	t := *base
	if d := th.dark(base); d != base.Dark {
		b := ui.LightTheme()
		if d {
			b = ui.DarkTheme()
		}
		b.FontSize, b.Font = base.FontSize, base.Font
		t = *b
	}
	t.Background, t.Surface, t.Border = p.Background, p.Raised, p.Border
	t.SurfaceHover, t.SurfacePressed = p.Track, p.Separator
	t.Text, t.TextMuted = p.Text, p.TextMuted
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = p.Accent, p.Accent, p.Accent, p.AccentText
	t.Danger = p.Danger
	t.Focus = ui.RGBA(p.Accent.R, p.Accent.G, p.Accent.B, 0.5)
	t.Selection = ui.RGBA(p.Accent.R, p.Accent.G, p.Accent.B, 0.3)
	t.Radius = th.radius()
	t.FontSize = th.fontSize()
	if th.Font != "" {
		t.Font = th.Font
	}
	return &t
}
