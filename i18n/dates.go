package i18n

import (
	"fmt"
	"time"
)

// DateTime formats a moment for lists and cards in the active language:
// day, month and time, with the year when it is not the current one
// ("Jan 2 15:04" / "Jan 2, 2025 15:04"; "1月2日 15:04" / "2025年1月2日 15:04"
// in Chinese and Japanese). Month names are English: Go's time package has
// no other.
func (c *Catalog) DateTime(t time.Time) string {
	t = t.Local()
	sameYear := t.Year() == c.now().Local().Year()
	if cjk(c.Locale()) {
		if sameYear {
			return fmt.Sprintf("%d月%d日 %s", t.Month(), t.Day(), t.Format("15:04"))
		}
		return fmt.Sprintf("%d年%d月%d日 %s", t.Year(), t.Month(), t.Day(), t.Format("15:04"))
	}
	if sameYear {
		return t.Format("Jan 2 15:04")
	}
	return t.Format("Jan 2, 2006 15:04")
}

// Date formats a day in the active language ("Jan 2, 2006" / "2006年1月2日").
func (c *Catalog) Date(t time.Time) string {
	t = t.Local()
	if cjk(c.Locale()) {
		return fmt.Sprintf("%d年%d月%d日", t.Year(), t.Month(), t.Day())
	}
	return t.Format("Jan 2, 2006")
}

// cjk reports a language that writes dates as 2006年1月2日.
func cjk(tag string) bool {
	b := base(tag)
	return b == "zh" || b == "ja"
}
