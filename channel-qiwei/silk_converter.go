package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/youthlin/silk"
)

const (
	silkSampleRate    = 24000
	silkBitsPerSample = 16
	silkChannels      = 1
)

func isSilkFormat(name string) bool {
	ext := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(ext, ".silk") || strings.HasSuffix(ext, ".slk")
}

func isSilkData(data []byte) bool {
	if len(data) < 10 {
		return false
	}
	header := string(data[:10])
	if strings.HasPrefix(header, "#!SILK_V3") {
		return true
	}
	if len(data) > 11 && data[0] == 0x02 && strings.HasPrefix(string(data[1:11]), "#!SILK_V3") {
		return true
	}
	return false
}

func decodeSilkToWav(silkData []byte) ([]byte, error) {
	pcm, err := silk.Decode(bytes.NewReader(silkData), silk.WithSampleRate(silkSampleRate))
	if err != nil {
		return nil, fmt.Errorf("silk decode: %w", err)
	}
	if len(pcm) == 0 {
		return nil, fmt.Errorf("silk decode produced empty pcm")
	}
	return wrapPCMAsWav(pcm, silkSampleRate, silkBitsPerSample, silkChannels)
}

func wrapPCMAsWav(pcm []byte, sampleRate, bitsPerSample, channels int) ([]byte, error) {
	dataLen := len(pcm)
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8

	var buf bytes.Buffer
	buf.Grow(44 + dataLen)

	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+dataLen))
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(channels))
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(&buf, binary.LittleEndian, uint32(byteRate))
	binary.Write(&buf, binary.LittleEndian, uint16(blockAlign))
	binary.Write(&buf, binary.LittleEndian, uint16(bitsPerSample))

	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataLen))
	buf.Write(pcm)

	return buf.Bytes(), nil
}

func replaceExtToWav(name string) string {
	for _, ext := range []string{".silk", ".slk", ".SILK", ".SLK"} {
		if strings.HasSuffix(name, ext) {
			return name[:len(name)-len(ext)] + ".wav"
		}
	}
	return name + ".wav"
}

func downloadRawBytes(a *app, url string) ([]byte, string, error) {
	resp, err := a.http.Get(url)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return raw, resp.Header.Get("Content-Type"), nil
}
