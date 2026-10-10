package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
)

type probeVerifier struct{ executable string }

type probeObservation struct {
	DurationSeconds      float64
	FirstVideoPTSSeconds float64
	LastVideoEndSeconds  float64
}

// exec writes stderr on one goroutine; it is inspected only after Wait joins it.
// Keep only its presence because a damaged long file can emit millions of errors.
type probeErrorOutput struct{ seen bool }

func (w *probeErrorOutput) Write(data []byte) (int, error) {
	if len(data) > 0 {
		w.seen = true
	}
	return len(data), nil
}

func (v probeVerifier) path() string {
	if v.executable != "" {
		return v.executable
	}
	return "ffprobe"
}
func (v probeVerifier) ready() error {
	if _, err := exec.LookPath(v.path()); err != nil {
		return errors.New("ffprobe is required for output validation; install FFmpeg or set ProbeExecutable")
	}
	return nil
}
func (v probeVerifier) verify(ctx context.Context, path string, length float64, width, height int) (probeObservation, error) {
	command := exec.CommandContext(ctx, v.path(), "-v", "error", "-select_streams", "v:0", "-count_frames", "-show_packets", "-show_entries", "format=duration:stream=codec_type,codec_name,width,height,nb_read_frames:packet=pts_time,duration_time", "-of", "json", path)
	var stderr probeErrorOutput
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		if ctx.Err() != nil {
			return probeObservation{}, ctx.Err()
		}
		return probeObservation{}, fmt.Errorf("ffprobe validation failed: %w", err)
	}
	if err = command.Start(); err != nil {
		if ctx.Err() != nil {
			return probeObservation{}, ctx.Err()
		}
		return probeObservation{}, fmt.Errorf("ffprobe validation failed: %w", err)
	}
	observation, probeErr := validateProbeReader(stdout, length, width, height)
	if probeErr != nil {
		// Stop ffprobe before waiting if the JSON is malformed or incomplete. It
		// may otherwise block forever writing to a pipe that we no longer read.
		killErr := command.Process.Kill()
		waitErr := command.Wait()
		if ctx.Err() != nil {
			return probeObservation{}, ctx.Err()
		}
		if errors.Is(killErr, os.ErrProcessDone) && waitErr != nil {
			return probeObservation{}, fmt.Errorf("ffprobe validation failed: %w", waitErr)
		}
		if stderr.seen {
			return probeObservation{}, errors.New("ffprobe reported decoding errors")
		}
		return probeObservation{}, probeErr
	}
	if err = command.Wait(); err != nil {
		if ctx.Err() != nil {
			return probeObservation{}, ctx.Err()
		}
		return probeObservation{}, fmt.Errorf("ffprobe validation failed: %w", err)
	}
	if stderr.seen {
		return probeObservation{}, errors.New("ffprobe reported decoding errors")
	}
	return observation, nil
}
func validateProbe(data []byte, length float64) error {
	_, err := validateProbeReader(bytes.NewReader(data), length, 1920, 1080)
	return err
}

// validateProbeReader consumes packets one at a time so ffprobe output memory
// stays bounded by a single record rather than the entire recording.
func validateProbeReader(r io.Reader, length float64, width, height int) (probeObservation, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	start, err := decoder.Token()
	if err != nil {
		return probeObservation{}, fmt.Errorf("ffprobe response: %w", err)
	}
	if delimiter, ok := start.(json.Delim); !ok || delimiter != '{' {
		return probeObservation{}, errors.New("ffprobe response: expected an object")
	}

	var containerDuration string
	var hasFormat, hasPackets, hasStreams, hasVideoFrames bool
	var observation probeObservation
	var hasVideo bool
	firstPTS, lastPacketEnd := math.Inf(1), math.Inf(-1)
	seen := map[string]bool{}
	for decoder.More() {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			return probeObservation{}, fmt.Errorf("ffprobe response: %w", tokenErr)
		}
		key, ok := token.(string)
		if !ok {
			return probeObservation{}, errors.New("ffprobe response: expected an object key")
		}
		if seen[key] {
			return probeObservation{}, fmt.Errorf("ffprobe response: duplicate field %q", key)
		}
		seen[key] = true
		switch key {
		case "programs":
			// ffprobe includes this empty section for native WebM even when the
			// selected show_entries contains only packet, stream and format fields.
			if err = expectProbeDelimiter(decoder, '['); err != nil {
				return probeObservation{}, err
			}
			if decoder.More() {
				return probeObservation{}, errors.New("ffprobe response: unexpected media program")
			}
			if err = expectProbeDelimiter(decoder, ']'); err != nil {
				return probeObservation{}, err
			}
		case "format":
			var format struct {
				Duration string `json:"duration"`
			}
			if err = decoder.Decode(&format); err != nil {
				return probeObservation{}, fmt.Errorf("ffprobe response: %w", err)
			}
			containerDuration = format.Duration
			hasFormat = true
		case "streams":
			if err = expectProbeDelimiter(decoder, '['); err != nil {
				return probeObservation{}, err
			}
			for decoder.More() {
				var stream struct {
					Type   string `json:"codec_type"`
					Codec  string `json:"codec_name"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
					Frames string `json:"nb_read_frames"`
				}
				if err = decoder.Decode(&stream); err != nil {
					return probeObservation{}, fmt.Errorf("ffprobe response: %w", err)
				}
				if stream.Type == "video" {
					hasVideo = true
					if stream.Codec != "vp9" || stream.Width != width || stream.Height != height {
						return probeObservation{}, fmt.Errorf("output video must be VP9 at %dx%d", width, height)
					}
					if frames, parseErr := strconv.ParseInt(stream.Frames, 10, 64); parseErr == nil && frames > 0 {
						hasVideoFrames = true
					}
				}
			}
			if err = expectProbeDelimiter(decoder, ']'); err != nil {
				return probeObservation{}, err
			}
			hasStreams = true
		case "packets":
			if err = expectProbeDelimiter(decoder, '['); err != nil {
				return probeObservation{}, err
			}
			for decoder.More() {
				var packet struct {
					PTS      string `json:"pts_time"`
					Duration string `json:"duration_time"`
				}
				if err = decoder.Decode(&packet); err != nil {
					return probeObservation{}, fmt.Errorf("ffprobe response: %w", err)
				}
				pts, ptsErr := strconv.ParseFloat(packet.PTS, 64)
				duration, durationErr := strconv.ParseFloat(packet.Duration, 64)
				packetEnd := pts + duration
				if ptsErr != nil || durationErr != nil || math.IsNaN(pts) || math.IsInf(pts, 0) || !finitePositive(duration) || math.IsNaN(packetEnd) || math.IsInf(packetEnd, 0) {
					return probeObservation{}, fmt.Errorf("%w: invalid packet timing pts=%q duration=%q", ErrRecordingIncomplete, packet.PTS, packet.Duration)
				}
				firstPTS = math.Min(firstPTS, pts)
				lastPacketEnd = math.Max(lastPacketEnd, packetEnd)
			}
			if err = expectProbeDelimiter(decoder, ']'); err != nil {
				return probeObservation{}, err
			}
			hasPackets = true
		default:
			return probeObservation{}, fmt.Errorf("ffprobe response: unexpected field %q", key)
		}
	}
	if err = expectProbeDelimiter(decoder, '}'); err != nil {
		return probeObservation{}, err
	}
	if err = requireProbeEOF(decoder, r); err != nil {
		return probeObservation{}, err
	}
	if !hasFormat || !hasStreams || !hasPackets {
		return probeObservation{}, fmt.Errorf("%w: ffprobe sections format=%t streams=%t packets=%t", ErrRecordingIncomplete, hasFormat, hasStreams, hasPackets)
	}
	if !hasVideo {
		return probeObservation{}, ErrRecordingIncomplete
	}

	duration, err := strconv.ParseFloat(containerDuration, 64)
	if err != nil || !finitePositive(duration) || math.Abs(duration-length) > math.Max(1, length*0.005) {
		return probeObservation{}, fmt.Errorf("%w: container duration=%q requested=%.3fs", ErrRecordingIncomplete, containerDuration, length)
	}
	observation.DurationSeconds = duration
	observation.FirstVideoPTSSeconds = firstPTS
	observation.LastVideoEndSeconds = lastPacketEnd
	// Audio can retain the requested length after the video ends early. Check
	// video packet timestamps, not frame count/FPS: native WebM is variable-rate.
	if math.Abs(firstPTS) > 0.25 || math.Abs(lastPacketEnd-length) > 0.5 {
		return probeObservation{}, fmt.Errorf("%w: video packet bounds firstPTS=%.6fs lastEnd=%.6fs requested=%.3fs", ErrRecordingIncomplete, firstPTS, lastPacketEnd, length)
	}
	if hasVideoFrames {
		return observation, nil
	}
	return probeObservation{}, errors.New("output has no decodable video frames")
}

func expectProbeDelimiter(decoder *json.Decoder, expected json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("ffprobe response: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != expected {
		return fmt.Errorf("ffprobe response: expected %q", expected)
	}
	return nil
}

func requireProbeEOF(decoder *json.Decoder, source io.Reader) error {
	// Decoder.Token would decode a trailing string into memory before rejecting it.
	// Inspect its small read-ahead buffer and the remaining stream as bytes instead.
	reader := io.MultiReader(decoder.Buffered(), source)
	var buffer [4096]byte
	for {
		n, err := reader.Read(buffer[:])
		for _, value := range buffer[:n] {
			if value != ' ' && value != '\t' && value != '\r' && value != '\n' {
				return errors.New("ffprobe response: trailing data")
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("ffprobe response: %w", err)
		}
	}
}
