package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type ContactGateway struct {
	accountID string
	repo      *contactRepo
	cache     *ttlCache
	src       ContactSource
}

func newContactGateway(db *sql.DB, accountID string, cache *ttlCache, src ContactSource) *ContactGateway {
	var repo *contactRepo
	if db != nil {
		repo = newContactRepo(db)
	}
	return &ContactGateway{
		accountID: accountID,
		repo:      repo,
		cache:     cache,
		src:       src,
	}
}

func (g *ContactGateway) ResolveName(ctx context.Context, userID, roomID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", nil
	}
	if g.cache != nil {
		if value, ok := g.cache.Get(userID); ok {
			return value, nil
		}
	}
	if g.repo != nil {
		contact, err := g.repo.GetContact(ctx, g.accountID, userID)
		if err == nil {
			name := preferredContactName(contact)
			g.cacheContactName(contact)
			return name, nil
		}
		if !errors.Is(err, ErrContactNotFound) {
			return "", err
		}
	}

	contact, err := g.fetchContact(ctx, userID)
	if err == nil {
		name := preferredContactName(contact)
		g.cacheContactName(contact)
		return name, nil
	}
	if !errors.Is(err, ErrContactNotFound) {
		return "", err
	}
	if strings.TrimSpace(roomID) == "" {
		return "", nil
	}
	room, err := g.ResolveRoom(ctx, roomID)
	if err != nil {
		return "", err
	}
	for _, member := range room.Members {
		if member.UserID == userID {
			if member.DisplayName != "" && g.cache != nil {
				g.cache.Set(userID, member.DisplayName)
			}
			return member.DisplayName, nil
		}
	}
	return "", nil
}

func (g *ContactGateway) ResolveContact(ctx context.Context, userID string) (Contact, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Contact{}, ErrContactNotFound
	}
	if g.repo != nil {
		contact, err := g.repo.GetContact(ctx, g.accountID, userID)
		if err == nil {
			g.cacheContactName(contact)
			return contact, nil
		}
		if !errors.Is(err, ErrContactNotFound) {
			return Contact{}, err
		}
	}
	return g.fetchContact(ctx, userID)
}

func (g *ContactGateway) ResolveRoom(ctx context.Context, roomID string) (RoomSnapshot, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return RoomSnapshot{}, ErrRoomNotFound
	}
	if g.repo != nil {
		room, err := g.repo.GetRoom(ctx, g.accountID, roomID)
		if err == nil {
			if g.cache != nil && room.Name != "" {
				g.cache.Set("room:"+roomID, room.Name)
			}
			members, listErr := g.repo.ListRoomMembers(ctx, g.accountID, roomID)
			if listErr != nil {
				return RoomSnapshot{}, listErr
			}
			return roomSnapshotFromRecord(room, members), nil
		}
		if !errors.Is(err, ErrRoomNotFound) {
			return RoomSnapshot{}, err
		}
	}
	if g.src == nil {
		return RoomSnapshot{}, ErrRoomNotFound
	}
	rooms, err := g.src.BatchGetRoomDetail(ctx, []string{roomID})
	if err != nil {
		return RoomSnapshot{}, err
	}
	for _, room := range rooms {
		if strings.TrimSpace(room.RoomID) != roomID {
			continue
		}
		if err := g.persistRoom(ctx, room); err != nil {
			return RoomSnapshot{}, err
		}
		return room, nil
	}
	return RoomSnapshot{}, ErrRoomNotFound
}

func (g *ContactGateway) ResolveExternalUserID(ctx context.Context, userID string) (string, error) {
	contact, err := g.ResolveContact(ctx, userID)
	if err != nil {
		return "", err
	}
	if contact.ExternalUserID != "" {
		return contact.ExternalUserID, nil
	}
	if g.src == nil {
		return "", nil
	}
	openID, _, err := g.src.GetOpenID(ctx, userID)
	if err != nil || openID == "" {
		return openID, err
	}
	contact.ExternalUserID = openID
	contact.LastSyncedAt = time.Now().Unix()
	contact.UpdatedAt = contact.LastSyncedAt
	if g.repo != nil {
		if err := g.repo.UpsertContact(ctx, contact); err != nil {
			return "", err
		}
	}
	return openID, nil
}

func (g *ContactGateway) HasContact(ctx context.Context, userID string) bool {
	if g == nil || g.repo == nil || strings.TrimSpace(userID) == "" {
		return false
	}
	_, err := g.repo.GetContact(ctx, g.accountID, userID)
	return err == nil
}

func (g *ContactGateway) SyncContacts(ctx context.Context) error {
	if g.src == nil {
		return nil
	}
	external, err := g.src.ListExternalContacts(ctx)
	if err != nil {
		return err
	}
	internal, err := g.src.ListInternalContacts(ctx)
	if err != nil {
		return err
	}
	for _, snapshot := range append(external, internal...) {
		if err := g.persistContact(ctx, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (g *ContactGateway) fetchContact(ctx context.Context, userID string) (Contact, error) {
	if g.src == nil {
		return Contact{}, ErrContactNotFound
	}
	contacts, err := g.src.BatchGetUserInfo(ctx, []string{userID})
	if err != nil {
		return Contact{}, err
	}
	for _, snapshot := range contacts {
		if strings.TrimSpace(snapshot.UserID) != userID {
			continue
		}
		if err := g.persistContact(ctx, snapshot); err != nil {
			return Contact{}, err
		}
		if g.repo != nil {
			return g.repo.GetContact(ctx, g.accountID, userID)
		}
		return contactFromSnapshot(g.accountID, snapshot, time.Now()), nil
	}
	return Contact{}, ErrContactNotFound
}

func (g *ContactGateway) persistContact(ctx context.Context, snapshot ContactSnapshot) error {
	if g.repo == nil {
		return nil
	}
	contact := contactFromSnapshot(g.accountID, snapshot, time.Now())
	if err := g.repo.UpsertContact(ctx, contact); err != nil {
		return err
	}
	g.cacheContactName(contact)
	return nil
}

func (g *ContactGateway) persistRoom(ctx context.Context, snapshot RoomSnapshot) error {
	if g.repo == nil {
		return nil
	}
	now := time.Now()
	room := roomFromSnapshot(g.accountID, snapshot, now)
	if err := g.repo.UpsertRoom(ctx, room); err != nil {
		return err
	}
	if err := g.repo.ReplaceRoomMembers(ctx, g.accountID, snapshot.RoomID, roomMembersFromSnapshot(g.accountID, snapshot, now)); err != nil {
		return err
	}
	if g.cache != nil && snapshot.Name != "" {
		g.cache.Set("room:"+snapshot.RoomID, snapshot.Name)
	}
	return nil
}

func (g *ContactGateway) cacheContactName(contact Contact) {
	if g.cache == nil {
		return
	}
	name := preferredContactName(contact)
	if name != "" {
		g.cache.Set(contact.UserID, name)
	}
}

func preferredContactName(contact Contact) string {
	return firstNonEmpty(contact.Remark, contact.RealName, contact.Nickname, contact.Alias, contact.UserID)
}

func roomSnapshotFromRecord(room Room, members []RoomMember) RoomSnapshot {
	snapshot := RoomSnapshot{
		RoomID:       room.RoomID,
		Name:         room.Name,
		Announcement: room.Announcement,
		Notice:       room.Notice,
		OwnerUserID:  room.OwnerUserID,
		MemberCount:  room.MemberCount,
		QRCodeURL:    room.QRCodeURL,
	}
	snapshot.Members = make([]RoomMemberSnapshot, 0, len(members))
	for _, member := range members {
		snapshot.Members = append(snapshot.Members, RoomMemberSnapshot{
			UserID:      member.UserID,
			DisplayName: member.DisplayName,
			Role:        member.Role,
			JoinedAt:    member.JoinedAt,
			LastSeenAt:  member.LastSeenAt,
		})
	}
	return snapshot
}
