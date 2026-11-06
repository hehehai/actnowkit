package hanzi

import "testing"

func TestConvert(t *testing.T) {
	const hant, hans = "最近我又見到一個很久沒見的老同學了", "最近我又见到一个很久没见的老同学了"
	if !HasTraditionalOnly(hant) || HasTraditionalOnly(hans) || HasSimplifiedOnly(hant) || !HasSimplifiedOnly(hans) {
		t.Error("Has…Only")
	}
	if got := ToSimplified(hant); got != hans {
		t.Errorf("ToSimplified = %q", got)
	}
	if got := ToTraditional("这个视频"); got != "這個視頻" {
		t.Errorf("ToTraditional = %q", got)
	}
	// Characters both scripts write are neither's: 著名, 乾坤, 之後.
	const shared = "著名的乾坤之後"
	if HasTraditionalOnly(shared) || HasSimplifiedOnly(shared) || ToSimplified(shared) != shared || ToTraditional(shared) != shared {
		t.Error("shared characters")
	}
	if ToSimplified("Hello, world") != "Hello, world" || ToTraditional("") != "" {
		t.Error("not Chinese")
	}
}

func TestRunes(t *testing.T) {
	for in, want := range map[rune]rune{'們': '们', '臺': '台', '裡': '里', '後': '後', '我': '我', 'a': 'a'} {
		if got := Simplified(in); got != want {
			t.Errorf("Simplified(%c) = %c, want %c", in, got, want)
		}
	}
	for in, want := range map[rune]rune{'们': '們', '这': '這', '後': '後', '我': '我', 'a': 'a'} {
		if got := Traditional(in); got != want {
			t.Errorf("Traditional(%c) = %c, want %c", in, got, want)
		}
	}
	if !TraditionalOnly('們') || TraditionalOnly('们') || !SimplifiedOnly('们') || SimplifiedOnly('們') || TraditionalOnly('後') {
		t.Error("…Only")
	}
}

// The tables are pairs of one character only one script writes and its
// form in the other script, which that script writes.
func TestTables(t *testing.T) {
	toHans, toHant := maps()
	if len([]rune(traditionalOnly))%2 != 0 || len([]rune(simplifiedOnly))%2 != 0 || len(toHans) < 1000 || len(toHant) < 1000 {
		t.Fatalf("tables: %d %d", len(toHans), len(toHant))
	}
	for from, to := range toHans {
		if from == to || TraditionalOnly(to) {
			t.Errorf("%c → %c", from, to)
		}
	}
	for from, to := range toHant {
		if from == to || SimplifiedOnly(to) {
			t.Errorf("%c → %c", from, to)
		}
	}
}
