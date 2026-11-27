package x11

import "testing"

func TestParseWmctrl(t *testing.T) {
	out := "0x01e00003  0 host Terminal\n0x00a00002 -1 host Desktop\ngarbage\n"
	ws := parseWmctrl(out)
	if len(ws) != 2 || ws[0] != (Window{0x01e00003, 0}) || ws[1] != (Window{0x00a00002, -1}) {
		t.Fatalf("got %+v", ws)
	}
}

func TestParseMonitor(t *testing.T) {
	out := "Monitors: 2\n 0: +*DP-1 2560/597x1440/336+0+0  DP-1\n 1: +HDMI-1 1920/527x1080/296+2560+180  HDMI-1\n"
	if r, ok := parseMonitor(out, 1); !ok || r != (Rect{2560, 180, 1920, 1080}) {
		t.Fatalf("monitor 1 = %+v %v", r, ok)
	}
	if _, ok := parseMonitor(out, 2); ok {
		t.Fatal("monitor 2 should not exist")
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
