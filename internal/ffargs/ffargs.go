// Package ffargs builds the ffmpeg command line for `periscope send`.
package ffargs

import (
	"fmt"
	"os"
	"strconv"

	"github.com/crl-sdn-vpz/periscope/internal/config"
	"github.com/crl-sdn-vpz/periscope/internal/srt"
)

// ServiceName tags the MPEG-TS stream; it also makes a periscope ffmpeg easy to spot in a process list.
const ServiceName = "periscope"

// Capture resolves "auto" to the capture backend for goos.
func Capture(capture, goos string) (string, error) {
	if capture != "auto" {
		return capture, nil
	}
	switch goos {
	case "windows":
		return "ddagrab", nil
	case "linux":
		return "x11grab", nil
	}
	return "", fmt.Errorf("no screen capture for %s yet (planned for milestone 3); set send.capture to \"test\" to try the pipeline", goos)
}

// Build returns ffmpeg's arguments (without the binary) for c on goos.
func Build(c config.Config, goos string) ([]string, error) {
	s := c.Send
	capture, err := Capture(s.Capture, goos)
	if err != nil {
		return nil, err
	}
	fps := strconv.Itoa(s.Fps)
	q := strconv.Itoa(s.Quality)
	gop := strconv.Itoa(s.Fps * 2)

	use444 := s.Chroma444 && (s.Encoder == "x264" || s.Encoder == "nvenc")
	pix := "nv12"
	switch {
	case use444:
		pix = "yuv444p"
	case s.Encoder == "x264":
		pix = "yuv420p"
	}

	a := []string{"-hide_banner", "-loglevel", "warning", "-nostdin"}
	switch capture {
	case "ddagrab":
		src := fmt.Sprintf("ddagrab=output_idx=%d:framerate=%s:draw_mouse=1,hwdownload,format=bgra", s.Monitor, fps)
		a = append(a, "-f", "lavfi", "-i", src)
	case "gdigrab":
		a = append(a, "-f", "gdigrab", "-framerate", fps, "-draw_mouse", "1", "-i", "desktop")
	case "x11grab":
		display := os.Getenv("DISPLAY")
		if display == "" {
			return nil, fmt.Errorf("x11grab needs an X11 session (DISPLAY is empty)")
		}
		a = append(a, "-f", "x11grab", "-framerate", fps, "-draw_mouse", "1", "-i", display)
	case "test":
		// A moving pattern with a clock at a fixed 2560x1440, so max_width scaling is exercised too.
		a = append(a, "-re", "-f", "lavfi", "-i", "testsrc2=size=2560x1440:rate="+fps)
	default:
		return nil, fmt.Errorf("unknown capture %q", capture)
	}

	vf := ""
	if s.MaxWidth > 0 {
		vf = fmt.Sprintf("scale=w='min(%d,iw)':h=-2:flags=lanczos,", s.MaxWidth)
	}
	a = append(a, "-vf", vf+"format="+pix, "-an")

	switch s.Encoder {
	case "nvenc":
		a = append(a, "-c:v", "h264_nvenc", "-preset", "p1", "-tune", "ull", "-zerolatency", "1",
			"-rc", "constqp", "-qp", q, "-g", gop, "-bf", "0")
		if use444 {
			a = append(a, "-profile:v", "high444p")
		}
	case "qsv":
		a = append(a, "-c:v", "h264_qsv", "-preset", "veryfast", "-global_quality", q,
			"-look_ahead", "0", "-async_depth", "1", "-g", gop, "-bf", "0")
	case "amf":
		a = append(a, "-c:v", "h264_amf", "-usage", "ultralowlatency", "-rc", "cqp",
			"-qp_i", q, "-qp_p", q, "-g", gop, "-bf", "0")
	default:
		a = append(a, "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
			"-crf", q, "-g", gop, "-bf", "0")
	}

	url := srt.URL("caller", s.TargetHost, c.Port, c.LatencyMs, c.Passphrase) + "&connect_timeout=3000"
	a = append(a, "-f", "mpegts", "-flush_packets", "1", "-muxdelay", "0",
		"-metadata", "service_name="+ServiceName, url)
	return a, nil
}
