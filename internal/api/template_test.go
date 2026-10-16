package api

import "testing"

func TestMsgTemplates(t *testing.T) {
	tm := newMsgTemplates()
	s := `{"n":{{i}},"c":"{{seq "a" "b"}}","id":"{{uuid}}","r":{{randInt 5 5}}}`
	f, err := tm.parse(&s)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i, want := range []string{"a", "b", "a"} {
		tm.i = i
		b, err := f()
		if err != nil {
			t.Fatal(err)
		}
		got := string(b)
		if got[:len(`{"n":0,"c":"a"`)] != `{"n":`+string(rune('0'+i))+`,"c":"`+want+`"` || got[len(got)-6:] != `"r":5}` {
			t.Fatalf("record %d: %s", i, got)
		}
		ids = append(ids, got[19:55])
	}
	if ids[0] == ids[1] {
		t.Fatal("uuid not unique per record")
	}
	if nilF, _ := tm.parse(nil); nilF != nil {
		if b, _ := nilF(); b != nil {
			t.Fatal("nil key must stay nil")
		}
	}
	bad := `{{pick}}`
	f, _ = tm.parse(&bad)
	if _, err := f(); err == nil {
		t.Fatal("pick with no args should error, not panic")
	}
}
