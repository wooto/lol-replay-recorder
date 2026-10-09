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
func (v probeVerifier) verify(ctx context.Context, path string, length float64) error {
	command := exec.CommandContext(ctx, v.path(), "-v", "error", "-select_streams", "v:0", "-count_frames", "-show_packets", "-show_entries", "format=duration:stream=codec_type,nb_read_frames:packet=pts_time,duration_time", "-of", "json", path)
	var stderr probeErrorOutput
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffprobe validation failed: %w", err)
	}
	if err = command.Start(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffprobe validation failed: %w", err)
	}
	probeErr := validateProbeReader(stdout, length)
	if probeErr != nil {
		// Stop ffprobe before waiting if the JSON is malformed or incomplete. It
		// may otherwise block forever writing to a pipe that we no longer read.
		killErr := command.Process.Kill()
		waitErr := command.Wait()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(killErr, os.ErrProcessDone) && waitErr != nil {
			return fmt.Errorf("ffprobe validation failed: %w", waitErr)
		}
		if stderr.seen {
			return errors.New("ffprobe reported decoding errors")
		}
		return probeErr
	}
	if err = command.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffprobe validation failed: %w", err)
	}
	if stderr.seen {
		return errors.New("ffprobe reported decoding errors")
	}
	return nil
}
func validateProbe(data []byte, length float64) error {
	return validateProbeReader(bytes.NewReader(data), length)
}

// validateProbeReader consumes packets one at a time so ffprobe output memory
// stays bounded by a single record rather than the entire recording.
func validateProbeReader(r io.Reader, length float64) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	start, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("ffprobe response: %w", err)
	}
	if delimiter, ok := start.(json.Delim); !ok || delimiter != '{' {
		return errors.New("ffprobe response: expected an object")
	}

	var containerDuration string
	var hasFormat, hasPackets, hasStreams, hasVideoFrames bool
	firstPTS, lastPacketEnd := math.Inf(1), math.Inf(-1)
	seen := map[string]bool{}
	for decoder.More() {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			return fmt.Errorf("ffprobe response: %w", tokenErr)
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("ffprobe response: expected an object key")
		}
		if seen[key] {
			return fmt.Errorf("ffprobe response: duplicate field %q", key)
		}
		seen[key] = true
		switch key {
		case "programs":
			// ffprobe includes this empty section for native WebM even when the
			// selected show_entries contains only packet, stream and format fields.
			if err = expectProbeDelimiter(decoder, '['); err != nil {
				return err
			}
			if decoder.More() {
				return errors.New("ffprobe response: unexpected media program")
			}
			if err = expectProbeDelimiter(decoder, ']'); err != nil {
				return err
			}
		case "format":
			var format struct {
				Duration string `json:"duration"`
			}
			if err = decoder.Decode(&format); err != nil {
				return fmt.Errorf("ffprobe response: %w", err)
			}
			containerDuration = format.Duration
			hasFormat = true
		case "streams":
			if err = expectProbeDelimiter(decoder, '['); err != nil {
				return err
			}
			for decoder.More() {
				var stream struct {
					Type   string `json:"codec_type"`
					Frames string `json:"nb_read_frames"`
				}
				if err = decoder.Decode(&stream); err != nil {
					return fmt.Errorf("ffprobe response: %w", err)
				}
				if stream.Type == "video" {
					if frames, parseErr := strconv.ParseInt(stream.Frames, 10, 64); parseErr == nil && frames > 0 {
						hasVideoFrames = true
					}
				}
			}
			if err = expectProbeDelimiter(decoder, ']'); err != nil {
				return err
			}
			hasStreams = true
		case "packets":
			if err = expectProbeDelimiter(decoder, '['); err != nil {
				return err
			}
			for decoder.More() {
				var packet struct {
					PTS      string `json:"pts_time"`
					Duration string `json:"duration_time"`
				}
				if err = decoder.Decode(&packet); err != nil {
					return fmt.Errorf("ffprobe response: %w", err)
				}
				pts, ptsErr := strconv.ParseFloat(packet.PTS, 64)
				duration, durationErr := strconv.ParseFloat(packet.Duration, 64)
				packetEnd := pts + duration
				if ptsErr != nil || durationErr != nil || math.IsNaN(pts) || math.IsInf(pts, 0) || !finitePositive(duration) || math.IsNaN(packetEnd) || math.IsInf(packetEnd, 0) {
					return ErrRecordingIncomplete
				}
				firstPTS = math.Min(firstPTS, pts)
				lastPacketEnd = math.Max(lastPacketEnd, packetEnd)
			}
			if err = expectProbeDelimiter(decoder, ']'); err != nil {
				return err
			}
			hasPackets = true
		default:
			return fmt.Errorf("ffprobe response: unexpected field %q", key)
		}
	}
	if err = expectProbeDelimiter(decoder, '}'); err != nil {
		return err
	}
	if err = requireProbeEOF(decoder, r); err != nil {
		return err
	}
	if !hasFormat || !hasStreams || !hasPackets {
		return ErrRecordingIncomplete
	}

	duration, err := strconv.ParseFloat(containerDuration, 64)
	if err != nil || !finitePositive(duration) || math.Abs(duration-length) > math.Max(1, length*0.005) {
		return ErrRecordingIncomplete
	}
	// Audio can retain the requested length after the video ends early. Check
	// video packet timestamps, not frame count/FPS: native WebM is variable-rate.
	if math.Abs(firstPTS) > 0.25 || math.Abs(lastPacketEnd-length) > 0.5 {
		return ErrRecordingIncomplete
	}
	if hasVideoFrames {
		return nil
	}
	return errors.New("output has no decodable video frames")
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
