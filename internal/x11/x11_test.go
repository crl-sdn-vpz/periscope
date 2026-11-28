package x11

import "testing"

func TestParseWmctrl(t *testing.T) {
	out := "0x01e00003  0 10 20 300 200 host Terminal window\n0x00a00002 -1 0 0 2560 1440 host Desktop\ngarbage\n"
	ws := parseWmctrl(out)
	want := []Window{{0x01e00003, 0, Rect{10, 20, 300, 200}}, {0x00a00002, -1, Rect{0, 0, 2560, 1440}}}
	if len(ws) != 2 || ws[0] != want[0] || ws[1] != want[1] {
		t.Fatalf("got %+v", ws)
	}
}

func TestParseMonitors(t *testing.T) {
	out := "Monitors: 2\n 0: +*DP-1 2560/597x1440/336+0+0  DP-1\n 1: +HDMI-1 1920/527x1080/296+2560+180  HDMI-1\n"
	ms := parseMonitors(out)
	want := []Monitor{{0, "DP-1", true, Rect{0, 0, 2560, 1440}}, {1, "HDMI-1", false, Rect{2560, 180, 1920, 1080}}}
	if len(ms) != 2 || ms[0] != want[0] || ms[1] != want[1] {
		t.Fatalf("got %+v", ms)
	}
	// Virtual monitors made with `xrandr --setmonitor` have no '+' and no output name suffix.
	ms = parseMonitors("Monitors: 1\n 0: left 1280/0x1440/0+0+0  none\n")
	if len(ms) != 1 || ms[0].Name != "left" || ms[0].Rect != (Rect{0, 0, 1280, 1440}) {
		t.Fatalf("virtual monitor: %+v", ms)
	}
	if !(Rect{2560, 180, 1920, 1080}).Contains(3000, 500) || (Rect{0, 0, 10, 10}).Contains(10, 5) {
		t.Fatal("Contains")
	}
}

func TestCardinalMatchesBashViewer(t *testing.T) {
	// e2e-xvfb.sh expects 3435973836 for 0.80 (the bash viewer's awk output).
	if got := Cardinal(0.80); got != 3435973836 {
		t.Fatalf("Cardinal(0.8) = %d", got)
	}
	if Cardinal(2) != 0xffffffff || Cardinal(-1) != 0 {
		t.Fatal("Cardinal must clamp to [0,1]")
	}
}
