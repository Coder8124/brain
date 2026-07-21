// Package record captures a study session and turns it into notes.
//
// This is the tutor's "record a lecture, get study material" loop — the thing a
// student most wants — done locally. While recording, it samples the screen and
// reads it with on-device OCR; on stop, the model titles the session, writes
// notes, and saves them as supporting material the tutor can then quiz from. If
// ffmpeg is available it also saves the raw video alongside, but the notes are
// the point and do not depend on it.
package record

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pragun/brain/internal/capture/sources"
)

// FrameInterval is how often the screen is sampled while recording. Slides and
// notes do not change second to second, so a few seconds captures the content
// without drowning the summariser in near-identical frames.
const FrameInterval = 4 * time.Second

// Frame is one sampled moment of the session.
type Frame struct {
	TS   int64
	Text string
}

// Recorder samples the screen on an interval until stopped. Safe for the app to
// start from one goroutine and stop from another.
type Recorder struct {
	scratch string
	mu      sync.Mutex
	frames  []Frame
	started int64
	stop    chan struct{}
	running bool

	// video, populated only if ffmpeg is present.
	ffmpeg    *exec.Cmd
	ffmpegIn  io.WriteCloser
	videoPath string
}

func NewRecorder(scratch string) *Recorder {
	return &Recorder{scratch: scratch}
}

func (r *Recorder) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// Start begins a session. withVideo attempts an ffmpeg screen capture in
// parallel; it is a no-op (notes still work) when ffmpeg is missing.
func (r *Recorder) Start(withVideo bool) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("already recording")
	}
	r.frames = nil
	r.started = time.Now().Unix()
	r.stop = make(chan struct{})
	r.running = true
	r.mu.Unlock()

	if withVideo {
		r.startVideo() // best-effort; ignores failure
	}

	go r.loop()
	return nil
}

func (r *Recorder) loop() {
	// Capture an immediate first frame so a short recording still has content.
	r.sample()
	t := time.NewTicker(FrameInterval)
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
			r.sample()
		}
	}
}

func (r *Recorder) sample() {
	text, err := sources.CaptureScreenText(r.scratch)
	if err != nil || strings.TrimSpace(text) == "" {
		return
	}
	r.mu.Lock()
	r.frames = append(r.frames, Frame{TS: time.Now().Unix(), Text: text})
	r.mu.Unlock()
}

// Session is the captured material, ready to be processed into notes.
type Session struct {
	Started   int64
	Ended     int64
	Frames    []Frame
	VideoPath string
}

// Stop ends the session and returns what was captured.
func (r *Recorder) Stop() *Session {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return nil
	}
	r.running = false
	close(r.stop)
	frames := r.frames
	video := r.videoPath
	r.mu.Unlock()

	r.stopVideo()

	return &Session{
		Started:   r.started,
		Ended:     time.Now().Unix(),
		Frames:    frames,
		VideoPath: video,
	}
}

// DedupedText concatenates the frames with consecutive duplicates collapsed.
//
// Sampling a static slide for two minutes yields thirty identical frames; the
// summariser wants the content once. Frames are compared on a normalised prefix
// so minor OCR jitter (a caret, a cursor) does not defeat the dedup.
func (s *Session) DedupedText() string {
	var b strings.Builder
	var prevKey string
	for _, f := range s.Frames {
		key := normalize(f.Text)
		if key == prevKey {
			continue
		}
		prevKey = key
		b.WriteString(strings.TrimSpace(f.Text))
		b.WriteString("\n\n---\n\n")
	}
	return strings.TrimSuffix(b.String(), "\n\n---\n\n")
}

func normalize(s string) string {
	s = strings.ToLower(s)
	fields := strings.Fields(s)
	if len(fields) > 40 {
		fields = fields[:40] // compare on the first ~40 words
	}
	return strings.Join(fields, " ")
}

func (s *Session) DurationMin() int {
	return int((s.Ended - s.Started) / 60)
}

func (s *Session) Empty() bool {
	return strings.TrimSpace(s.DedupedText()) == ""
}

// --- video via ffmpeg (optional) ---

// FFmpegAvailable reports whether video capture is possible.
func FFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

func (r *Recorder) startVideo() {
	if !FFmpegAvailable() {
		return
	}
	if err := os.MkdirAll(r.scratch, 0o755); err != nil {
		return
	}
	out := filepath.Join(r.scratch, fmt.Sprintf("rec-%d.mp4", r.started))

	// The avfoundation index of "Capture screen" varies by machine — cameras
	// enumerate first — so discover it rather than hardcode. Fall back to a
	// common value if detection fails.
	screen := screenDeviceIndex()

	// 4 fps: this is study content, not gameplay — small files, low CPU.
	cmd := exec.Command("ffmpeg", "-y", "-f", "avfoundation", "-framerate", "4",
		"-i", screen+":none", "-vcodec", "h264", "-pix_fmt", "yuv420p", out)

	// The stdin pipe must be wired before Start; it is how we later send ffmpeg
	// a graceful "q" so the .mp4 is finalised with a valid moov atom rather than
	// truncated by a kill.
	in, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}
	r.mu.Lock()
	r.ffmpeg = cmd
	r.ffmpegIn = in
	r.videoPath = out
	r.mu.Unlock()
}

func (r *Recorder) stopVideo() {
	r.mu.Lock()
	cmd := r.ffmpeg
	in := r.ffmpegIn
	r.ffmpeg, r.ffmpegIn = nil, nil
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	// ffmpeg finalises the file cleanly on 'q'; the pipe was opened at Start.
	if in != nil {
		in.Write([]byte("q"))
		in.Close()
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
	}
}

// screenDeviceIndex parses ffmpeg's avfoundation device list for the "Capture
// screen" entry and returns its index. The index differs per machine because
// video input devices (cameras) are enumerated before displays.
func screenDeviceIndex() string {
	out, err := exec.Command("ffmpeg", "-f", "avfoundation", "-list_devices", "true", "-i", "").CombinedOutput()
	if err == nil || len(out) > 0 {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(strings.ToLower(line), "capture screen") {
				// line looks like: [AVFoundation indev @ 0x..] [4] Capture screen 0
				if i := strings.Index(line, "] ["); i >= 0 {
					rest := line[i+3:]
					if j := strings.Index(rest, "]"); j > 0 {
						return strings.TrimSpace(rest[:j])
					}
				}
			}
		}
	}
	return "1" // best-effort fallback
}
