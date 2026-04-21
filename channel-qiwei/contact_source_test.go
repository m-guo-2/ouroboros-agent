package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestQiWeProtoSourceMapsExternalContactFields(t *testing.T) {
	src := &qiweProtoSource{
		call: func(_ context.Context, method string, _ map[string]any) (qiweiDoAPIResponse, error) {
			if method != "/contact/getWxContactList" {
				t.Fatalf("unexpected method %s", method)
			}
			data := map[string]any{
				"contactList": []map[string]any{{
					"userId":    "user-1",
					"openid":    "external-1",
					"nickname":  "VGVzdGVy",
					"realName":  "QWxpY2U=",
					"alias":     "YWxpY2U=",
					"remark":    "dmlw",
					"avatarUrl": "https://img.example.com/a.png",
					"corpId":    "corp-1",
					"corpName":  "QUNNRSBDb3Jw",
					"gender":    "2",
				}},
			}
			raw, _ := json.Marshal(data)
			return qiweiDoAPIResponse{Code: 0, Msg: "ok", Data: raw}, nil
		},
	}

	contacts, err := src.ListExternalContacts(context.Background())
	if err != nil {
		t.Fatalf("ListExternalContacts: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected one contact, got %d", len(contacts))
	}
	got := contacts[0]
	if got.UserID != "user-1" || got.ExternalUserID != "external-1" {
		t.Fatalf("unexpected identity mapping: %+v", got)
	}
	if got.Source != "external" {
		t.Fatalf("expected external source, got %q", got.Source)
	}
	if got.Nickname != "Tester" || got.RealName != "Alice" || got.Alias != "alice" || got.Remark != "vip" {
		t.Fatalf("expected decoded display fields, got %+v", got)
	}
	if got.CorpName != "ACME Corp" {
		t.Fatalf("expected decoded corp name, got %q", got.CorpName)
	}
	if got.Raw["openid"] != "external-1" {
		t.Fatalf("expected raw payload preserved, got %+v", got.Raw)
	}
}

func TestQiWeProtoSourceMapsRoomDetails(t *testing.T) {
	src := &qiweProtoSource{
		call: func(_ context.Context, method string, params map[string]any) (qiweiDoAPIResponse, error) {
			if method != "/room/batchGetRoomDetail" {
				t.Fatalf("unexpected method %s", method)
			}
			if len(params["roomIdList"].([]string)) != 1 {
				t.Fatalf("unexpected params: %+v", params)
			}
			data := map[string]any{
				"roomList": []map[string]any{{
					"roomId":       "room-1",
					"roomName":     "56+k6K+V576k",
					"announcement": "5YWs5ZGK",
					"notice":       "5aSH5rOo",
					"createUserId": "owner-1",
					"memberCount":  2,
					"memberList": []map[string]any{
						{"userId": "owner-1", "displayName": "576k5Li7"},
						{"userId": "member-1", "name": "5oiQ5ZGYQQ=="},
					},
				}},
			}
			raw, _ := json.Marshal(data)
			return qiweiDoAPIResponse{Code: 0, Msg: "ok", Data: raw}, nil
		},
	}

	rooms, err := src.BatchGetRoomDetail(context.Background(), []string{"room-1"})
	if err != nil {
		t.Fatalf("BatchGetRoomDetail: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("expected one room, got %d", len(rooms))
	}
	room := rooms[0]
	if room.RoomID != "room-1" || room.OwnerUserID != "owner-1" || room.MemberCount != 2 {
		t.Fatalf("unexpected room mapping: %+v", room)
	}
	if len(room.Members) != 2 {
		t.Fatalf("expected two members, got %+v", room.Members)
	}
	if room.Members[0].Role != "owner" || room.Members[1].Role != "member" {
		t.Fatalf("unexpected member roles: %+v", room.Members)
	}
	if room.Raw["roomId"] != "room-1" {
		t.Fatalf("expected raw room payload preserved, got %+v", room.Raw)
	}
}
