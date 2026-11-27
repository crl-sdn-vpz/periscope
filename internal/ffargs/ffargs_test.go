package ffargs

import (
	"strings"
	"testing"

	"github.com/crl-sdn-vpz/periscope/internal/config"
)

func cfg() config.Config {
	c := config.Default()
	c.Passphrase = "abcdefghij0123456789"
	c.Send.TargetHost = "192.168.1.50"
	return c
}

// The default Windows command must stay identical to what windows/sender.ps1 (Build-Args) produces.
func TestWindowsDefaultMatchesPowerShellSender(t *testing.T) {
	got, err := Build(cfg(), "windows")
	if err != nil {
		t.Fatal(err)
	}
	want := "-hide_banner -loglevel warning -nostdin " +
		"-f lavfi -i ddagrab=output_idx=0:framerate=30:draw_mouse=1,hwdownload,format=bgra " +
		"-vf format=yuv444p -an " +
		"-c:v libx264 -preset ultrafast -tune zerolatency -crf 18 -g 60 -bf 0 " +
		"-f mpegts -flush_packets 1 -muxdelay 0 -metadata service_name=periscope " +
		"srt://192.168.1.50:9000?mode=caller&latency=80000&pbkeylen=16&passphrase=abcdefghij0123456789&connect_timeout=3000"
	if s := strings.Join(got, " "); s != want {
		t.Fatalf("got\n %s\nwant\n %s", s, want)
	}
}

func TestEncoders(t *testing.T) {
	cases := []struct {
		encoder string
		chroma  bool
		want    []string // fragments that must appear
		absent  string
	}{
		{"nvenc", true, []string{"format=yuv444p", "-c:v h264_nvenc", "-qp 18", "-profile:v high444p"}, ""},
		{"nvenc", false, []string{"format=nv12", "-c:v h264_nvenc"}, "high444p"},
		{"qsv", true, []string{"format=nv12", "-c:v h264_qsv", "-global_quality 18"}, "yuv444p"},
		{"amf", true, []string{"format=nv12", "-c:v h264_amf", "-qp_i 18 -qp_p 18"}, "yuv444p"},
		{"x264", false, []string{"format=yuv420p", "-c:v libx264"}, ""},
	}
	for _, tc := range cases {
		c := cfg()
		c.Send.Encoder, c.Send.Chroma444 = tc.encoder, tc.chroma
		got, err := Build(c, "windows")
		if err != nil {
			t.Fatal(err)
		}
		s := strings.Join(got, " ")
		for _, w := range tc.want {
			if !strings.Contains(s, w) {
				t.Errorf("%s/444=%v: missing %q in %s", tc.encoder, tc.chroma, w, s)
			}
		}
		if tc.absent != "" && strings.Contains(s, tc.absent) {
			t.Errorf("%s/444=%v: unexpected %q", tc.encoder, tc.chroma, tc.absent)
		}
	}
}

func TestCaptureAndScale(t *testing.T) {
	c := cfg()
	c.Send.Capture, c.Send.MaxWidth, c.Send.Fps = "gdigrab", 1920, 20
	got, _ := Build(c, "windows")
	s := strings.Join(got, " ")
	for _, w := range []string{"-f gdigrab -framerate 20 -draw_mouse 1 -i desktop",
		"-vf scale=w='min(1920,iw)':h=-2:flags=lanczos,format=yuv444p", "-g 40"} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %q in %s", w, s)
		}
	}
	if _, err := Build(cfg(), "darwin"); err == nil {
		t.Error("auto capture on darwin should be refused until milestone 3")
	}
	t.Setenv("DISPLAY", ":7")
	got, _ = Build(cfg(), "linux")
	if s := strings.Join(got, " "); !strings.Contains(s, "-f x11grab -framerate 30 -draw_mouse 1 -i :7") {
		t.Errorf("linux auto capture: %s", s)
	}
}
