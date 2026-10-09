package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
)

type probeVerifier struct{ executable string }

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
	command := exec.CommandContext(ctx, v.path(), "-v", "error", "-count_frames", "-show_entries", "format=duration:stream=codec_type,nb_read_frames", "-of", "json", path)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	data, err := command.Output()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffprobe validation failed: %w", err)
	}
	if stderr.Len() > 0 {
		return errors.New("ffprobe reported decoding errors")
	}
	return validateProbe(data, length)
}
func validateProbe(data []byte, length float64) error {
	var probe struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type   string `json:"codec_type"`
			Frames string `json:"nb_read_frames"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("ffprobe response: %w", err)
	}
	duration, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil || !finitePositive(duration) || math.Abs(duration-length) > math.Max(1, length*0.005) {
		return ErrRecordingIncomplete
	}
	for _, stream := range probe.Streams {
		if frames, e := strconv.ParseInt(stream.Frames, 10, 64); e == nil && frames > 0 && stream.Type == "video" {
			return nil
		}
	}
	return errors.New("output has no decodable video frames")
}
