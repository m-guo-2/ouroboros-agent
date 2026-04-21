package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ContactSource defines the data-source boundary for contact/room identity
// snapshots. Today we only implement qiweProtoSource (QiWe third-party
// protocol); a future officialAPISource should implement the same methods so
// the repo schema and gateway HTTP contract stay unchanged. When both sources
// exist, ContactGateway should prefer officialAPISource for GetOpenID-style
// lookups and use qiweProtoSource as the default list/detail source until the
// official integration is complete.
type ContactSource interface {
	ListExternalContacts(ctx context.Context) ([]ContactSnapshot, error)
	ListInternalContacts(ctx context.Context) ([]ContactSnapshot, error)
	ListRooms(ctx context.Context) ([]RoomSnapshot, error)
	BatchGetRoomDetail(ctx context.Context, roomIDs []string) ([]RoomSnapshot, error)
	BatchGetUserInfo(ctx context.Context, userIDs []string) ([]ContactSnapshot, error)
	GetOpenID(ctx context.Context, userID string) (openid, unionid string, err error)
}

type ContactSnapshot struct {
	UserID         string
	ExternalUserID string
	Source         string
	Nickname       string
	RealName       string
	Alias          string
	Remark         string
	AvatarURL      string
	Gender         string
	CorpID         string
	CorpName       string
	FollowUser     any
	Raw            map[string]any
}

type RoomSnapshot struct {
	RoomID       string
	Name         string
	Announcement string
	Notice       string
	OwnerUserID  string
	MemberCount  int64
	QRCodeURL    string
	Members      []RoomMemberSnapshot
	Raw          map[string]any
}

type RoomMemberSnapshot struct {
	UserID      string
	DisplayName string
	Role        string
	JoinedAt    int64
	LastSeenAt  int64
}

type doAPICaller func(ctx context.Context, method string, params map[string]any) (qiweiDoAPIResponse, error)

type qiweProtoSource struct {
	call doAPICaller
}

func newQiWeProtoSource(client *qiweiClient) *qiweProtoSource {
	return &qiweProtoSource{call: client.doAPIRaw}
}

func (s *qiweProtoSource) ListExternalContacts(ctx context.Context) ([]ContactSnapshot, error) {
	return s.listContacts(ctx, "/contact/getWxContactList", "external", nil)
}

func (s *qiweProtoSource) ListInternalContacts(ctx context.Context) ([]ContactSnapshot, error) {
	return s.listContacts(ctx, "/contact/getWxWorkContactList", "internal", nil)
}

func (s *qiweProtoSource) BatchGetUserInfo(ctx context.Context, userIDs []string) ([]ContactSnapshot, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	return s.listContacts(ctx, "/contact/batchGetUserinfo", "unknown", map[string]any{
		"userIdList": userIDs,
	})
}

func (s *qiweProtoSource) ListRooms(ctx context.Context) ([]RoomSnapshot, error) {
	res, err := s.call(ctx, "/room/getRoomList", nil)
	if err != nil {
		return nil, err
	}
	data, err := decodeAPIData(res.Data)
	if err != nil {
		return nil, err
	}
	items := extractItems(data, "roomList", "list", "rows", "data")
	rooms := make([]RoomSnapshot, 0, len(items))
	for _, item := range items {
		rooms = append(rooms, toRoomSnapshot(item))
	}
	return rooms, nil
}

func (s *qiweProtoSource) BatchGetRoomDetail(ctx context.Context, roomIDs []string) ([]RoomSnapshot, error) {
	if len(roomIDs) == 0 {
		return nil, nil
	}
	res, err := s.call(ctx, "/room/batchGetRoomDetail", map[string]any{
		"roomIdList": roomIDs,
	})
	if err != nil {
		return nil, err
	}
	data, err := decodeAPIData(res.Data)
	if err != nil {
		return nil, err
	}
	items := extractItems(data, "roomList", "list", "rows", "data")
	rooms := make([]RoomSnapshot, 0, len(items))
	for _, item := range items {
		rooms = append(rooms, toRoomSnapshot(item))
	}
	return rooms, nil
}

func (s *qiweProtoSource) GetOpenID(ctx context.Context, userID string) (string, string, error) {
	res, err := s.call(ctx, "/contact/openid", map[string]any{"userId": userID})
	if err != nil {
		return "", "", err
	}
	var data map[string]any
	if err := unmarshalSafe(res.Data, &data); err != nil {
		return "", "", err
	}
	return anyToString(data["openid"]), anyToString(data["unionid"]), nil
}

func (s *qiweProtoSource) listContacts(ctx context.Context, method, source string, params map[string]any) ([]ContactSnapshot, error) {
	res, err := s.call(ctx, method, params)
	if err != nil {
		return nil, err
	}
	data, err := decodeAPIData(res.Data)
	if err != nil {
		return nil, err
	}
	items := extractItems(data, "contactList", "list", "rows", "data")
	out := make([]ContactSnapshot, 0, len(items))
	for _, item := range items {
		out = append(out, toContactSnapshot(item, source))
	}
	return out, nil
}

func toContactSnapshot(item map[string]any, source string) ContactSnapshot {
	return ContactSnapshot{
		UserID:         anyToString(item["userId"]),
		ExternalUserID: anyToString(item["openid"]),
		Source:         firstNonEmpty(source, "unknown"),
		Nickname:       decodeMaybeBase64(anyToString(item["nickname"])),
		RealName:       decodeMaybeBase64(anyToString(item["realName"])),
		Alias:          decodeMaybeBase64(anyToString(item["alias"])),
		Remark:         decodeMaybeBase64(anyToString(item["remark"])),
		AvatarURL:      anyToString(item["avatarUrl"]),
		Gender:         anyToString(item["gender"]),
		CorpID:         anyToString(item["corpId"]),
		CorpName:       decodeMaybeBase64(anyToString(item["corpName"])),
		FollowUser:     item["followUser"],
		Raw:            cloneMap(item),
	}
}

func toRoomSnapshot(item map[string]any) RoomSnapshot {
	room := RoomSnapshot{
		RoomID:       anyToString(item["roomId"]),
		Name:         decodeMaybeBase64(anyToString(item["roomName"])),
		Announcement: decodeMaybeBase64(anyToString(item["announcement"])),
		Notice:       decodeMaybeBase64(anyToString(item["notice"])),
		OwnerUserID:  firstNonEmpty(anyToString(item["ownerUserId"]), anyToString(item["createUserId"])),
		MemberCount:  anyToInt64(item["memberCount"]),
		QRCodeURL:    firstNonEmpty(anyToString(item["qrCodeUrl"]), anyToString(item["roomQrCode"])),
		Raw:          cloneMap(item),
	}
	rawMembers, _ := item["memberList"].([]any)
	room.Members = make([]RoomMemberSnapshot, 0, len(rawMembers))
	for _, raw := range rawMembers {
		member, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := "member"
		userID := anyToString(member["userId"])
		if userID != "" && userID == room.OwnerUserID {
			role = "owner"
		}
		room.Members = append(room.Members, RoomMemberSnapshot{
			UserID:      userID,
			DisplayName: decodeMaybeBase64(firstNonEmpty(anyToString(member["displayName"]), anyToString(member["name"]))),
			Role:        role,
			JoinedAt:    anyToInt64(member["joinTime"]),
			LastSeenAt:  anyToInt64(member["lastSeenAt"]),
		})
	}
	return room
}

func rawMapJSON(raw map[string]any) string {
	if len(raw) == 0 {
		return "{}"
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func followUserJSON(value any) string {
	if value == nil {
		return "[]"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	encoded, err := json.Marshal(in)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func contactFromSnapshot(accountID string, snapshot ContactSnapshot, now time.Time) Contact {
	ts := now.Unix()
	return Contact{
		AccountID:      strings.TrimSpace(accountID),
		UserID:         strings.TrimSpace(snapshot.UserID),
		ExternalUserID: strings.TrimSpace(snapshot.ExternalUserID),
		Source:         firstNonEmpty(snapshot.Source, "unknown"),
		Nickname:       snapshot.Nickname,
		RealName:       snapshot.RealName,
		Alias:          snapshot.Alias,
		Remark:         snapshot.Remark,
		AvatarURL:      snapshot.AvatarURL,
		Gender:         snapshot.Gender,
		CorpID:         snapshot.CorpID,
		CorpName:       snapshot.CorpName,
		FollowUserJSON: followUserJSON(snapshot.FollowUser),
		RawJSON:        rawMapJSON(snapshot.Raw),
		FirstSeenAt:    ts,
		LastSyncedAt:   ts,
		UpdatedAt:      ts,
	}
}

func roomFromSnapshot(accountID string, snapshot RoomSnapshot, now time.Time) Room {
	ts := now.Unix()
	return Room{
		AccountID:    strings.TrimSpace(accountID),
		RoomID:       strings.TrimSpace(snapshot.RoomID),
		Name:         snapshot.Name,
		Announcement: snapshot.Announcement,
		Notice:       snapshot.Notice,
		OwnerUserID:  snapshot.OwnerUserID,
		MemberCount:  snapshot.MemberCount,
		QRCodeURL:    snapshot.QRCodeURL,
		RawJSON:      rawMapJSON(snapshot.Raw),
		FirstSeenAt:  ts,
		LastSyncedAt: ts,
		UpdatedAt:    ts,
	}
}

func roomMembersFromSnapshot(accountID string, snapshot RoomSnapshot, now time.Time) []RoomMember {
	out := make([]RoomMember, 0, len(snapshot.Members))
	ts := now.Unix()
	for _, member := range snapshot.Members {
		if strings.TrimSpace(member.UserID) == "" {
			continue
		}
		lastSeenAt := member.LastSeenAt
		if lastSeenAt == 0 {
			lastSeenAt = ts
		}
		out = append(out, RoomMember{
			AccountID:   accountID,
			RoomID:      snapshot.RoomID,
			UserID:      member.UserID,
			DisplayName: member.DisplayName,
			Role:        firstNonEmpty(member.Role, "member"),
			JoinedAt:    member.JoinedAt,
			LastSeenAt:  lastSeenAt,
		})
	}
	return out
}

func requireUserIDs(userIDs []string) error {
	for _, userID := range userIDs {
		if strings.TrimSpace(userID) == "" {
			return fmt.Errorf("userIdList contains empty user id")
		}
	}
	return nil
}
