package redact

import "testing"

func TestPersonal(t *testing.T) {
	p := Personal{Home: "/Users/ann", Names: []string{"ann", "Ann Lee", "Ann's MacBook Pro", "张三", "x"}, KeepEmails: []string{"me@example.com"}}
	cases := map[string]string{
		"open /Users/ann/Library/Logs/Dicta/dicta.log": "open ~/Library/Logs/Dicta/dicta.log",
		"home is /Users/ann":                           "home is ~",
		"other /Users/bob/Desktop and /Users/Shared/x": "other /Users/USER/Desktop and /Users/Shared/x",
		"user ann logged in; annual report by Ann Lee": "user USER logged in; annual report by USER",
		"host Ann's MacBook Pro.local":                 "host USER.local",
		"用户张三的文件":                                      "用户USER的文件",
		"mail ann.lee+x@gmail.com and me@example.com":  "mail a***@gmail.com and me@example.com",
		"from 203.0.113.42 to 127.0.0.1 and 0.0.0.0":   "from 203.0.x.x to 127.0.0.1 and 0.0.0.0",
		"v6 2001:db8:85a3::8a2e:370:7334 lo ::1":       "v6 IPv6 lo ::1",
		"at 12:34:56 version 1.2.3 x":                  "at 12:34:56 version 1.2.3 x",
		"":                                             "",
	}
	for in, want := range cases {
		if got := p.Text(in); got != want {
			t.Errorf("Text(%q) = %q, want %q", in, got, want)
		}
	}
	if got := (Personal{}).Text("a@b.io at /Users/zed"); got != "a***@b.io at /Users/USER" {
		t.Errorf("zero: %q", got)
	}
	_ = ThisUser("Mac")
}
