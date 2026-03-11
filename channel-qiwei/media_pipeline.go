package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

type mediaSource string

const (
	mediaSourceUnknown mediaSource = "unknown"
	mediaSourceQW      mediaSource = "qw"
	mediaSourceGW      mediaSource = "gw"
)

type mediaKind string

const (
	mediaKindUnknown mediaKind = "unknown"
	mediaKindImage   mediaKind = "image"
	mediaKindFile    mediaKind = "file"
	mediaKindVoice   mediaKind = "voice"
	mediaKindVideo   mediaKind = "video"
)

type mediaClassification struct {
	MsgType     int
	MessageType string
	Source      mediaSource
	Kind        mediaKind
}

type mediaDescriptor struct {
	Classification mediaClassification
	Name           string
	FileID         string
	FileAESKey     string
	FileAuthKey    string
	FileMD5        string
	FileSize       int64
	FileType       int
	CDNKey         string
	PreferredURL   string
	Raw            map[string]any
}

type mediaDownloadPlan struct {
	Name   string
	Method string
	Params map[string]any
}

type preparedMedia struct {
	MessageType string
	Content     string
	ResourceURI string
	LocalPath   string
	Name        string
	MIMEType    string
}

var mediaClassifications = map[int]mediaClassification{
	1:   {MsgType: 1, MessageType: "text", Source: mediaSourceUnknown, Kind: mediaKindUnknown},
	2:   {MsgType: 2, MessageType: "text", Source: mediaSourceUnknown, Kind: mediaKindUnknown},
	3:   {MsgType: 3, MessageType: "image", Source: mediaSourceQW, Kind: mediaKindImage},
	14:  {MsgType: 14, MessageType: "image", Source: mediaSourceQW, Kind: mediaKindImage},
	15:  {MsgType: 15, MessageType: "file", Source: mediaSourceQW, Kind: mediaKindFile},
	16:  {MsgType: 16, MessageType: "voice", Source: mediaSourceQW, Kind: mediaKindVoice},
	23:  {MsgType: 23, MessageType: "video", Source: mediaSourceQW, Kind: mediaKindVideo},
	34:  {MsgType: 34, MessageType: "voice", Source: mediaSourceQW, Kind: mediaKindVoice},
	43:  {MsgType: 43, MessageType: "video", Source: mediaSourceQW, Kind: mediaKindVideo},
	47:  {MsgType: 47, MessageType: "sticker", Source: mediaSourceUnknown, Kind: mediaKindUnknown},
	49:  {MsgType: 49, MessageType: "file", Source: mediaSourceQW, Kind: mediaKindFile},
	101: {MsgType: 101, MessageType: "image", Source: mediaSourceGW, Kind: mediaKindImage},
	102: {MsgType: 102, MessageType: "file", Source: mediaSourceGW, Kind: mediaKindFile},
	103: {MsgType: 103, MessageType: "video", Source: mediaSourceGW, Kind: mediaKindVideo},
	104: {MsgType: 104, MessageType: "sticker", Source: mediaSourceGW, Kind: mediaKindUnknown},
}

func classifyMediaMessage(msgType int, fallbackType string, msgData map[string]any) (mediaClassification, bool) {
	if cls, ok := mediaClassifications[msgType]; ok {
		return cls, true
	}

	messageType := strings.TrimSpace(fallbackType)
	if messageType == "" {
		return mediaClassification{}, false
	}
	switch messageType {
	case "text", "rich_text", "hyper_text":
		return mediaClassification{
			MsgType:     msgType,
			MessageType: messageType,
			Source:      mediaSourceUnknown,
			Kind:        mediaKindUnknown,
		}, true
	case "image":
		return mediaClassification{MsgType: msgType, MessageType: "image", Source: inferMediaSource(msgData), Kind: mediaKindImage}, true
	case "file":
		return mediaClassification{MsgType: msgType, MessageType: "file", Source: inferMediaSource(msgData), Kind: mediaKindFile}, true
	case "voice":
		return mediaClassification{MsgType: msgType, MessageType: "voice", Source: inferMediaSource(msgData), Kind: mediaKindVoice}, true
	case "video":
		return mediaClassification{MsgType: msgType, MessageType: "video", Source: inferMediaSource(msgData), Kind: mediaKindVideo}, true
	default:
		return mediaClassification{}, false
	}
}

func inferMediaSource(msgData map[string]any) mediaSource {
	if len(msgData) == 0 {
		return mediaSourceUnknown
	}
	if firstNonEmpty(
		anyToString(msgData["fileAuthKey"]),
		anyToString(msgData["fileAuthkey"]),
		anyToString(msgData["fileBigHttpUrl"]),
		anyToString(msgData["fileMiddleHttpUrl"]),
		anyToString(msgData["fileThumbHttpUrl"]),
		anyToString(msgData["fileHttpUrl"]),
	) != "" {
		return mediaSourceGW
	}
	if firstNonEmpty(
		anyToString(msgData["fileId"]),
		anyToString(msgData["fileID"]),
		anyToString(msgData["cdnKey"]),
		anyToString(msgData["cdn_key"]),
	) != "" {
		return mediaSourceQW
	}
	return mediaSourceUnknown
}

func normalizeMediaDescriptor(msgType int, fallbackType string, msgData map[string]any) (mediaDescriptor, error) {
	classification, ok := classifyMediaMessage(msgType, fallbackType, msgData)
	if !ok {
		return mediaDescriptor{}, fmt.Errorf("unsupported media message: msgType=%d fallbackType=%s", msgType, fallbackType)
	}

	name := firstNonEmpty(
		decodeMaybeBase64(anyToString(msgData["fileName"])),
		anyToString(msgData["name"]),
		decodeMaybeBase64(anyToString(msgData["fileNameUtf8"])),
	)

	desc := mediaDescriptor{
		Classification: classification,
		Name:           name,
		FileID:         firstNonEmpty(anyToString(msgData["fileId"]), anyToString(msgData["fileID"])),
		FileAESKey:     firstNonEmpty(anyToString(msgData["fileAesKey"]), anyToString(msgData["fileAeskey"]), anyToString(msgData["fileAESKey"])),
		FileAuthKey:    firstNonEmpty(anyToString(msgData["fileAuthKey"]), anyToString(msgData["fileAuthkey"])),
		FileMD5:        firstNonEmpty(anyToString(msgData["fileMd5"]), anyToString(msgData["fileMD5"]), anyToString(msgData["md5"])),
		FileSize: firstNonZero(
			anyToInt64(msgData["fileSize"]),
			anyToInt64(msgData["fileBigSize"]),
			anyToInt64(msgData["fileMiddleSize"]),
			anyToInt64(msgData["fileThumbSize"]),
			anyToInt64(msgData["size"]),
		),
		CDNKey: firstNonEmpty(anyToString(msgData["cdnKey"]), anyToString(msgData["cdn_key"])),
		Raw:    msgData,
	}
	desc.PreferredURL = preferredMediaURL(desc.Classification, msgData)
	desc.FileType = inferMediaContractFileType(desc.Classification, msgData)
	if desc.Name == "" {
		desc.Name = inferredAttachmentName(string(desc.Classification.Kind), desc.PreferredURL)
	}
	return desc, nil
}

func preferredMediaURL(classification mediaClassification, msgData map[string]any) string {
	switch classification.Source {
	case mediaSourceGW:
		switch classification.Kind {
		case mediaKindImage:
			return firstNonEmpty(
				anyToString(msgData["fileBigHttpUrl"]),
				anyToString(msgData["fileMiddleHttpUrl"]),
				anyToString(msgData["fileThumbHttpUrl"]),
				anyToString(msgData["fileHttpUrl"]),
			)
		case mediaKindVideo, mediaKindFile, mediaKindVoice:
			return firstNonEmpty(anyToString(msgData["fileHttpUrl"]), anyToString(msgData["voiceUrl"]))
		}
	case mediaSourceQW:
		return firstNonEmpty(
			anyToString(msgData["url"]),
			anyToString(msgData["fileUrl"]),
			anyToString(msgData["imageUrl"]),
			anyToString(msgData["imgUrl"]),
			anyToString(msgData["voiceUrl"]),
		)
	}
	return firstNonEmpty(
		anyToString(msgData["url"]),
		anyToString(msgData["fileUrl"]),
		anyToString(msgData["fileBigHttpUrl"]),
		anyToString(msgData["fileMiddleHttpUrl"]),
		anyToString(msgData["fileThumbHttpUrl"]),
		anyToString(msgData["fileHttpUrl"]),
		anyToString(msgData["imgUrl"]),
		anyToString(msgData["imageUrl"]),
		anyToString(msgData["voiceUrl"]),
	)
}

func inferMediaContractFileType(classification mediaClassification, msgData map[string]any) int {
	if v := int(anyToInt64(msgData["fileType"])); v > 0 {
		return v
	}
	switch classification.Kind {
	case mediaKindVoice, mediaKindFile:
		return 5
	case mediaKindVideo:
		return 4
	case mediaKindImage:
		if firstNonEmpty(anyToString(msgData["fileThumbHttpUrl"]), anyToString(msgData["thumb"])) != "" &&
			firstNonEmpty(anyToString(msgData["fileBigHttpUrl"]), anyToString(msgData["fileMiddleHttpUrl"])) == "" {
			return 3
		}
		if firstNonEmpty(anyToString(msgData["fileBigHttpUrl"]), anyToString(msgData["bigImgUrl"])) != "" {
			return 1
		}
		if anyToInt64(msgData["imageHasHd"]) > 0 || anyToInt64(msgData["image_has_hd"]) > 0 {
			return 1
		}
		return 2
	default:
		return 0
	}
}

func planMediaDownload(desc mediaDescriptor) (mediaDownloadPlan, error) {
	switch desc.Classification.Source {
	case mediaSourceQW:
		if desc.CDNKey != "" {
			return mediaDownloadPlan{
				Name:   "qw-cdn-key",
				Method: "/cloud/cdnWxDownload",
				Params: map[string]any{"cdnKey": desc.CDNKey},
			}, nil
		}
		if desc.FileID != "" && desc.FileAESKey != "" && desc.FileMD5 != "" &&
			(desc.Classification.Kind == mediaKindImage || desc.Classification.Kind == mediaKindVideo) {
			return mediaDownloadPlan{
				Name:   "qw-cdn-file",
				Method: "/cloud/cdnWxDownload",
				Params: map[string]any{"fileId": desc.FileID, "fileAeskey": desc.FileAESKey, "fileMd5": desc.FileMD5},
			}, nil
		}
		if desc.FileID != "" && desc.FileAESKey != "" && desc.FileSize > 0 && desc.FileType > 0 {
			return mediaDownloadPlan{
				Name:   "qw-cloud-file",
				Method: "/cloud/wxWorkDownload",
				Params: map[string]any{"fileId": desc.FileID, "fileAeskey": desc.FileAESKey, "fileSize": desc.FileSize, "fileType": desc.FileType},
			}, nil
		}
		if desc.PreferredURL != "" && !looksLikeProtectedDownloadURL(desc.PreferredURL) {
			return mediaDownloadPlan{Name: "direct", Method: "DIRECT", Params: map[string]any{"url": desc.PreferredURL}}, nil
		}
	case mediaSourceGW:
		if desc.FileAESKey != "" && desc.FileAuthKey != "" && desc.FileSize > 0 && desc.FileType > 0 && desc.PreferredURL != "" {
			return mediaDownloadPlan{
				Name:   "gw-cloud-file",
				Method: "/cloud/wxDownload",
				Params: map[string]any{"fileAeskey": desc.FileAESKey, "fileAuthkey": desc.FileAuthKey, "fileSize": desc.FileSize, "fileType": desc.FileType, "fileUrl": desc.PreferredURL},
			}, nil
		}
		if desc.PreferredURL != "" && !looksLikeProtectedDownloadURL(desc.PreferredURL) {
			return mediaDownloadPlan{Name: "direct", Method: "DIRECT", Params: map[string]any{"url": desc.PreferredURL}}, nil
		}
	}
	if desc.PreferredURL != "" && !looksLikeProtectedDownloadURL(desc.PreferredURL) {
		return mediaDownloadPlan{Name: "direct", Method: "DIRECT", Params: map[string]any{"url": desc.PreferredURL}}, nil
	}
	return mediaDownloadPlan{}, fmt.Errorf("no valid media download plan")
}

func looksLikeProtectedDownloadURL(raw string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return false
	}
	return strings.Contains(raw, "imunion.weixin.qq.com/cgi-bin/mmae-bin/tpdownloadmedia")
}

func (a *app) prepareMediaForAgent(ctx context.Context, msgType int, fallbackType string, msgData map[string]any) preparedMedia {
	classification, ok := classifyMediaMessage(msgType, fallbackType, msgData)
	if !ok || classification.Kind == mediaKindUnknown {
		return preparedMedia{
			MessageType: firstNonEmpty(fallbackType, "unknown"),
			Content:     strings.TrimSpace(firstNonEmpty(anyToString(msgData["content"]), "[收到消息]")),
		}
	}

	desc, err := normalizeMediaDescriptor(msgType, fallbackType, msgData)
	if err != nil {
		logger.Warn(ctx, "媒体处理失败",
			"stage", "normalize",
			"msgType", msgType,
			"messageType", classification.MessageType,
			"source", string(classification.Source),
			"kind", string(classification.Kind),
			"error", err.Error(),
		)
		return preparedMedia{MessageType: classification.MessageType, Content: mediaPlaceholder(classification)}
	}

	plan, err := planMediaDownload(desc)
	if err != nil {
		logger.Warn(ctx, "媒体处理失败",
			"stage", "plan",
			"msgType", msgType,
			"messageType", classification.MessageType,
			"source", string(classification.Source),
			"kind", string(classification.Kind),
			"error", err.Error(),
		)
		return preparedMedia{MessageType: classification.MessageType, Content: mediaPlaceholder(classification)}
	}

	logger.Business(ctx, "媒体下载计划",
		"msgType", msgType,
		"messageType", classification.MessageType,
		"source", string(classification.Source),
		"kind", string(classification.Kind),
		"strategy", plan.Name,
		"method", plan.Method,
	)

	resolvedURL, err := a.executeMediaDownloadPlan(ctx, plan)
	if err != nil {
		logger.Warn(ctx, "媒体处理失败",
			"stage", "resolve",
			"msgType", msgType,
			"messageType", classification.MessageType,
			"source", string(classification.Source),
			"kind", string(classification.Kind),
			"strategy", plan.Name,
			"method", plan.Method,
			"error", err.Error(),
		)
		return preparedMedia{MessageType: classification.MessageType, Content: mediaPlaceholder(classification)}
	}

	if classification.Kind == mediaKindVoice {
		transcript, err := a.transcribePreparedVoice(ctx, desc, resolvedURL)
		if err != nil {
			logger.Warn(ctx, "媒体处理失败",
				"stage", "transcribe",
				"msgType", msgType,
				"messageType", classification.MessageType,
				"source", string(classification.Source),
				"kind", string(classification.Kind),
				"strategy", plan.Name,
				"error", err.Error(),
			)
			return preparedMedia{MessageType: classification.MessageType, Content: mediaPlaceholder(classification)}
		}
		if strings.TrimSpace(transcript) == "" {
			return preparedMedia{MessageType: classification.MessageType, Content: mediaPlaceholder(classification)}
		}
		return preparedMedia{MessageType: classification.MessageType, Content: strings.TrimSpace(transcript), Name: desc.Name}
	}

	resourceURI, mimeType, err := a.materializePreparedMedia(ctx, desc, resolvedURL)
	if err != nil {
		logger.Warn(ctx, "媒体处理失败",
			"stage", "materialize",
			"msgType", msgType,
			"messageType", classification.MessageType,
			"source", string(classification.Source),
			"kind", string(classification.Kind),
			"strategy", plan.Name,
			"resolvedURL", resolvedURL,
			"error", err.Error(),
		)
		return preparedMedia{MessageType: classification.MessageType, Content: mediaPlaceholder(classification)}
	}

	return preparedMedia{
		MessageType: classification.MessageType,
		Content:     agentMediaContent(classification, resourceURI, desc.Name),
		ResourceURI: resourceURI,
		LocalPath:   resourceURI,
		Name:        desc.Name,
		MIMEType:    mimeType,
	}
}

func (a *app) executeMediaDownloadPlan(ctx context.Context, plan mediaDownloadPlan) (string, error) {
	if plan.Method == "DIRECT" {
		return anyToString(plan.Params["url"]), nil
	}
	res, err := a.client.doAPIRaw(ctx, plan.Method, plan.Params)
	if err != nil {
		return "", err
	}
	data, err := decodeAPIData(res.Data)
	if err != nil {
		return "", err
	}
	url := firstURL(data)
	if strings.TrimSpace(url) == "" {
		return "", fmt.Errorf("empty download url from %s", plan.Method)
	}
	return url, nil
}

func (a *app) materializePreparedMedia(ctx context.Context, desc mediaDescriptor, resolvedURL string) (string, string, error) {
	attachment := parsedAttachment{
		Kind:      internalAttachmentKind(desc.Classification.Kind),
		Name:      desc.Name,
		SourceURL: resolvedURL,
	}
	if attachment.Name != "" {
		attachment.MIMEType = mime.TypeByExtension(strings.ToLower(filepath.Ext(attachment.Name)))
	}
	return a.downloadAttachment(ctx, attachment)
}

func (a *app) transcribePreparedVoice(ctx context.Context, desc mediaDescriptor, resolvedURL string) (string, error) {
	name := strings.TrimSpace(desc.Name)
	if resolvedName := strings.TrimSpace(inferredAttachmentName("audio", resolvedURL)); resolvedName != "" {
		if name == "" || looksLikeGenericVoiceName(name) {
			name = resolvedName
		}
	}

	raw, contentType, err := downloadRawBytes(a, resolvedURL)
	if err != nil {
		return "", fmt.Errorf("download voice: %w", err)
	}

	audioData := raw
	audioName := name
	audioMIME := contentType

	if isSilkFormat(name) || isSilkData(raw) {
		wav, convErr := decodeSilkToWav(raw)
		if convErr != nil {
			return "", fmt.Errorf("silk to wav: %w", convErr)
		}
		audioData = wav
		audioName = replaceExtToWav(name)
		audioMIME = "audio/wav"
		logger.Business(ctx, "silk 转换为 wav", "originalName", name, "wavSize", len(wav))
	}

	attachment := parsedAttachment{
		Kind:     "audio",
		Name:     audioName,
		MIMEType: audioMIME,
	}
	publicURL, mimeType, err := a.uploadDownloadedAttachment(ctx, attachment, bytes.NewReader(audioData), audioMIME, int64(len(audioData)))
	if err != nil {
		return "", fmt.Errorf("upload voice: %w", err)
	}
	attachment.SourceURL = publicURL
	if strings.TrimSpace(mimeType) != "" {
		attachment.MIMEType = mimeType
	}

	taskID, err := a.recognizer.SubmitAudioTranscription(ctx, attachment)
	if err != nil {
		return "", err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		parsed, done, err := a.recognizer.QueryAudioTranscription(ctx, taskID)
		if err != nil {
			return "", err
		}
		if done {
			return strings.TrimSpace(parsed.ParsedText), nil
		}
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("audio transcription timed out")
}

func mediaPlaceholder(classification mediaClassification) string {
	switch classification.Kind {
	case mediaKindImage:
		return "[收到图片]"
	case mediaKindFile:
		return "[收到文件]"
	case mediaKindVideo:
		return "[收到视频]"
	case mediaKindVoice:
		return "[收到语音]"
	default:
		return "[收到消息]"
	}
}

func agentMediaContent(classification mediaClassification, resourceURI, name string) string {
	parts := []string{mediaPlaceholder(classification)}
	if strings.TrimSpace(name) != "" {
		parts = append(parts, fmt.Sprintf("名称: %s", strings.TrimSpace(name)))
	}
	if strings.TrimSpace(resourceURI) != "" {
		parts = append(parts, fmt.Sprintf("地址: %s", strings.TrimSpace(resourceURI)))
	}
	return strings.Join(parts, "\n")
}

func looksLikeGenericVoiceName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "", "voice.mp3", "attachment.dat", "file.dat":
		return true
	default:
		return false
	}
}

func internalAttachmentKind(kind mediaKind) string {
	switch kind {
	case mediaKindFile:
		return "document"
	case mediaKindVoice:
		return "audio"
	case mediaKindImage:
		return "image"
	default:
		return string(kind)
	}
}

func decodeMaybeBase64(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return raw
	}
	trimmed := strings.TrimSpace(string(decoded))
	if trimmed == "" {
		return raw
	}
	return trimmed
}
