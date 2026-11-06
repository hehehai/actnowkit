package transcheck

import (
	"strings"
	"testing"

	"github.com/hehehai/actnowkit/lang"
)

func parse(t *testing.T, tag string) lang.Tag {
	t.Helper()
	l, ok := lang.Parse(tag)
	if !ok {
		t.Fatalf("Parse(%q)", tag)
	}
	return l
}

// The lines of a session (CaptionCraft, 2026-10-16) whose translations
// came out one bubble late, and their right translations.
const (
	srcA = "今天我将独自骑着自行车，深入戈壁滩，去寻找一种。北江阿勒泰地区的自然瑰宝——呼伦湖金丝玉。"
	srcB = "我带了两天的补给。会在戈壁滩里面露营，然后去寻找那些漂亮的石头。"
	srcC = "这是一片无人的戈壁。"
	srcD = "然后需要走一些沙土路。"

	enA     = "Today I will cycle alone deep into the Gobi to look for a natural treasure of the Altai region: the golden jade of Hulun Lake."
	enARest = "The golden jade of Hulun Lake, a natural treasure of the Altai region."
	enB     = "I brought two days of supplies. I will camp in the Gobi and then look for those beautiful stones."
	enC     = "This is an uninhabited stretch of the Gobi."
	enD     = "Then we need to walk some dirt roads."
)

func TestUntranslated(t *testing.T) {
	en, zh := parse(t, "en"), parse(t, "zh-Hans")
	cases := []struct {
		src, out string
		tgt      lang.Tag
		want     bool
	}{
		{"预算还没定", "预算还没定", en, true},
		{"预算还没定", "The budget", en, false},
		{"Sounds good", "Sounds good", en, false},
		{"Sounds good", "Sounds good", zh, true},
		{"OK", "OK", zh, true},
		{"5", "5", zh, false},
		{"In the late 1880s, vendors sold ice cream.", "In the 1880s, street vendors sold ice cream.", zh, true},
		{"预算", "Budget", en, false},
		{"Bonjour à tous", "Hello everyone", en, false},
	}
	for _, c := range cases {
		if got := Untranslated(c.src, c.out, c.tgt); got != c.want {
			t.Errorf("%q→%q (%s): %v", c.src, c.out, c.tgt.Code, got)
		}
	}
}

// A line whose translation says much less than it (a model that split
// one line over two ids) is translated again on its own.
func TestCutShort(t *testing.T) {
	src4 := `Wealthy Mediterranean nobility sent laborers to trek up high mountains to harvest glacial ice and snow. Meanwhile, ancient Persians built shallow insulated pools of water and utilized a technique known as "sky cooling."`
	src5 := `At night, the shallow pools would naturally radiate heat into the dry desert skies, causing them to dip below the ambient temperature and freeze. Yet the cream-based treat we know today made a much later debut.`
	for _, c := range []struct {
		src, out string
		want     bool
	}{
		{src4, "富有的地中海贵族会派遣劳工到高山上采集冰川冰块和雪。", true},
		{src5, "与此同时，古代波斯人建造了浅浅的隔热水池，并利用了一种称为“天空降温”的技术。", true},
		{src4, "富有的地中海贵族派劳工翻山越岭去采集冰川冰和雪。与此同时，古代波斯人建造了浅浅的隔热水池，并利用一种称为“天空冷却”的技术。", false},
		{"可以，只要供应商愿意保持现在的价格。", "OK, as long as the supplier is willing to keep the current price.", false},
		{"好的。", "OK.", false},
		{"演示用的数据我们要提前准备好。我建议用真实的会议录音，效果更直观。但是要注意隐私，最好用我们自己的内部会议。", "We need to prepare the demo data in advance.", true},
		{"演示用的数据我们要提前准备好。我建议用真实的会议录音，效果更直观。", "We need the demo data ready in advance, and I suggest real meeting recordings since they are more vivid.", false},
		// Seen on the Mac (#18): a live caption with a stray full stop, its
		// first sentence translated only.
		{"我们这次项目复盘的主要结论是：由于前期需求没有和客户反复确认到。导致开发过程中多次返工，最终整体进度比原计划推迟了将近三个星期。",
			"The main conclusion from our project review this time is: because the requirements were not repeatedly confirmed with the client in advance.", true},
		{"我们这次项目复盘的主要结论是：由于前期需求没有和客户反复确认到。导致开发过程中多次返工，最终整体进度比原计划推迟了将近三个星期。",
			"The main conclusion of our project review is that because the requirements were not repeatedly confirmed with the client, development was reworked many times and the schedule slipped by almost three weeks.", false},
	} {
		if got := CutShort(c.src, c.out); got != c.want {
			t.Errorf("CutShort(%.30q, %.30q) = %v, want %v (units %.0f → %.0f, sentences %d → %d)", c.src, c.out, got, c.want,
				MeaningUnits(c.src), MeaningUnits(c.out), Sentences(c.src), Sentences(c.out))
		}
	}
}

func TestTooLongAndRepeats(t *testing.T) {
	if !TooLong(srcC, enB) || TooLong(srcC, enC) || TooLong(srcA, enA) || TooLong("大了。", "It's big.") || TooLong("嗯。", "Mm-hmm, right.") {
		t.Error("TooLong")
	}
	if !Repeats(srcB, enARest, srcA, enA) || Repeats(srcB, enB, srcA, enA) || !Repeats(srcC, enC+" "+enD, srcD, enD) {
		t.Error("Repeats")
	}
	if Repeats("好的。", "OK.", "好的。", "OK.") {
		t.Error("too short to tell")
	}
	// A speaker saying the same thing twice: the sources repeat too.
	if Repeats("I will camp in the Gobi tonight.", "我今晚会在戈壁露营。", "I will camp in the Gobi tonight.", "我今晚会在戈壁露营。") {
		t.Error("said twice")
	}
	// Each character of a script without spaces is a token (Thai here).
	if !Repeats("one", "สวัสดีครับ", "two", "สวัสดีครับ") {
		t.Error("Thai trigrams")
	}
}

func TestSentences(t *testing.T) {
	got := strings.Join(SplitSentences(srcA+" Then we go. It costs 1.5 yuan"), "|")
	if got != "今天我将独自骑着自行车，深入戈壁滩，去寻找一种。|北江阿勒泰地区的自然瑰宝——呼伦湖金丝玉。|Then we go.|It costs 1.5 yuan" {
		t.Fatal(got)
	}
	for s, want := range map[string]int{"": 1, "Hi": 1, "Hi there. Bye.": 2, "Ask Mr. Li!": 2, "The U.S. Army won.": 1, `He said "cool." Then left?`: 2, "好。好吗？好！": 3} {
		if got := Sentences(s); got != want {
			t.Errorf("Sentences(%q) = %d, want %d", s, got, want)
		}
	}
	if SplitSentences("  ") != nil {
		t.Error("empty")
	}
}

func TestUnits(t *testing.T) {
	for s, want := range map[string]float64{"": 0, "中文": 2, "abc": 1, "かなカナ한글": 6, "a1 b2 c3!": 2} {
		if got := MeaningUnits(s); got < want-1e-9 || got > want+1e-9 {
			t.Errorf("MeaningUnits(%q) = %v, want %v", s, got, want)
		}
	}
	if !ShortenedTooMuch("We need to finish this report by tomorrow morning.", "Report.") ||
		ShortenedTooMuch("We need to finish this report by tomorrow morning.", "Finish the report by morning.") {
		t.Error("ShortenedTooMuch")
	}
}

func TestWordless(t *testing.T) {
	for _, c := range []struct {
		src, out string
		want     bool
	}{
		{"So let's jump right in.", "1", true},
		{"Use this plus button.", "}1", true},
		{"Use this plus button.", "使用这个加号按钮。", false},
		{"5", "5", false},
		{"A", "", false},
	} {
		if got := Wordless(c.src, c.out); got != c.want {
			t.Errorf("Wordless(%q, %q) = %v", c.src, c.out, got)
		}
	}
}

func TestWrongScript(t *testing.T) {
	zh, en, ja := parse(t, "zh-Hans"), parse(t, "en"), parse(t, "ja")
	for _, c := range []struct {
		src, out string
		tgt      lang.Tag
		want     bool
	}{
		{"Sounds good", "Sounds great", zh, true},
		{"Sounds good", "听起来不错", zh, false},
		{"Sounds good", "OK 好", zh, false}, // a letter of the target's script is enough
		{"预算", "Budget", en, false},
		{"预算还没定", "预算还没有定", en, true},
		{"Hello there", "こんにちは", ja, false},
		{"Hello there", "123", zh, false}, // no letters: Wordless tells
	} {
		if got := WrongScript(c.src, c.out, c.tgt); got != c.want {
			t.Errorf("WrongScript(%q, %q, %s) = %v", c.src, c.out, c.tgt.Code, got)
		}
	}
}

func TestHanScript(t *testing.T) {
	hans, hant, en, yue := parse(t, "zh-Hans"), parse(t, "zh-TW"), parse(t, "en"), parse(t, "yue")
	if ChineseScript(hans) != "Hans" || ChineseScript(hant) != "Hant" || ChineseScript(yue) != "Hant" || ChineseScript(en) != "" {
		t.Error("ChineseScript")
	}
	if !OtherHan("最近我又見到一個很久沒見的老同學了", hans) || OtherHan("最近我又见到一个很久没见的老同学了", hans) {
		t.Error("Traditional in Simplified")
	}
	if got := ToScript("最近我又見到一個很久沒見的老同學了", hans); got != "最近我又见到一个很久没见的老同学了" {
		t.Errorf("to Simplified: %q", got)
	}
	// Characters both scripts write are neither's: 著名, 乾坤, 之後.
	if OtherHan("著名的乾坤之後", hans) || OtherHan("著名的乾坤之後", hant) {
		t.Error("shared characters")
	}
	if !OtherHan("这个视频", hant) || ToScript("这个视频", hant) != "這個視頻" {
		t.Errorf("to Traditional: %q", ToScript("这个视频", hant))
	}
	if OtherHan("見到", en) || ToScript("見到", en) != "見到" {
		t.Error("not Chinese")
	}
}

func TestAsItIs(t *testing.T) {
	for s, want := range map[string]bool{"Sam.": true, "Google DeepMind.": true, "Mm-hmm.": true, "": false, "  ": false,
		"Right here, in the middle.": false, "Supercalifragilisticexpialidocious!!": false} {
		if AsItIs(s) != want {
			t.Errorf("AsItIs(%q) = %v", s, !want)
		}
	}
}

func TestChecked(t *testing.T) {
	for _, c := range []struct {
		src, out, target, want string
		ok                     bool
	}{
		{"We need to finish this report.", "我們需要完成這份報告。", "zh-Hans", "我们需要完成这份报告。", true},
		{"We need to finish this report.", "我们需要完成这份报告。", "zh", "我们需要完成这份报告。", true},
		{"We need to finish this report by tomorrow.", "We need to finish this report by tomorrow.", "zh-Hans", "We need to finish this report by tomorrow.", false},
		{"Sam.", "Sam.", "zh-Hans", "Sam.", true},
		{"Hello there.", "  ", "ja", "", false},
		{"  ", "", "ja", "", true},
		{"今日は雨です。", "It is raining today.", "en", "It is raining today.", true},
		{"Hello", " Hallo ", "xx-nope", " Hallo ", true},
		{"Hello", " ", "xx-nope", " ", false},
	} {
		got, ok := Checked(c.src, c.out, c.target)
		if got != c.want || ok != c.ok {
			t.Errorf("Checked(%q, %q, %s) = %q, %v; want %q, %v", c.src, c.out, c.target, got, ok, c.want, c.ok)
		}
	}
}
