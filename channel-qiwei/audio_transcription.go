package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type preparedAudioForTranscription struct {
	Data      []byte
	Format    string
	Converted bool
}

var volcDirectAudioFormats = map[string]string{
	"wav":  "wav",
	"mp3":  "mp3",
	"ogg":  "ogg",
	"oga":  "ogg",
	"opus": "opus",
	"spx":  "spx",
	"amr":  "amr",
	"aac":  "aac",
	"m4a":  "m4a",
}

var audioFormatsConvertibleToWav = map[string]bool{
	"silk": true,
	"slk":  true,
	"flac": true,
	"webm": true,
	"weba": true,
}

func prepareAudioForTranscription(ctx context.Context, name string, raw []byte) (preparedAudioForTranscription, error) {
	if len(raw) == 0 {
		return preparedAudioForTranscription{}, fmt.Errorf("audio transcription requires audio data")
	}
	ext := audioExtFromName(name)
	if isSilkFormat(name) || isSilkData(raw) {
		wav, err := decodeSilkToWav(raw)
		if err != nil {
			return preparedAudioForTranscription{}, err
		}
		return preparedAudioForTranscription{Data: wav, Format: "wav", Converted: true}, nil
	}
	if format, ok := volcDirectAudioFormats[ext]; ok {
		return preparedAudioForTranscription{Data: raw, Format: format}, nil
	}
	if audioFormatsConvertibleToWav[ext] {
		wav, err := transcodeAudioToWav(ctx, raw)
		if err != nil {
			return preparedAudioForTranscription{}, err
		}
		return preparedAudioForTranscription{Data: wav, Format: "wav", Converted: true}, nil
	}
	return preparedAudioForTranscription{}, fmt.Errorf("unsupported audio format %q", ext)
}

func audioExtFromName(name string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(strings.TrimSpace(name))), ".")
}

func transcodeAudioToWav(ctx context.Context, raw []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0",
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-f", "wav",
		"pipe:1",
	)
	cmd.Stdin = bytes.NewReader(raw)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("ffmpeg audio transcode failed: %s", msg)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ffmpeg audio transcode produced empty wav")
	}
	return out, nil
}
