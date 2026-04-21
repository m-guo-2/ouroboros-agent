package main

import (
	"errors"
	"fmt"
	"strings"
)

// encodeConversationID assembles the transparent composite id that we emit
// to the agent: "{rawId}@{shortHash}". If shortHash is empty we fall back to
// the raw id alone (single-account deployments where short_hash was not
// computed yet).
func encodeConversationID(rawID, shortHash string) string {
	rawID = strings.TrimSpace(rawID)
	shortHash = strings.TrimSpace(shortHash)
	if rawID == "" {
		return ""
	}
	if shortHash == "" {
		return rawID
	}
	return rawID + "@" + shortHash
}

// decodeConversationID splits a composite id back to (rawId, shortHash).
// When the input has no "@" suffix, shortHash is "" — the caller is expected
// to fall back to the default runtime (single-account mode) or report an
// error.
func decodeConversationID(composite string) (rawID, shortHash string) {
	composite = strings.TrimSpace(composite)
	if composite == "" {
		return "", ""
	}
	idx := strings.LastIndex(composite, "@")
	if idx < 0 {
		return composite, ""
	}
	return composite[:idx], composite[idx+1:]
}

// ErrAmbiguousAccount signals that an outgoing request did not specify an
// account (no suffix, no account_id) while more than one account is
// registered — the caller MUST pick explicitly.
var ErrAmbiguousAccount = errors.New("multiple qiwei accounts are registered; specify channelConversationId suffix or account_id")

// ErrUnknownAccount signals that a suffix / account_id did not match any
// known account.
var ErrUnknownAccount = errors.New("qiwei account not found for the given routing key")

// resolveOutgoingTarget maps a composite channelConversationId to the target
// runtime and the raw qiwei id we should hand to /msg/send*. The rules are:
//
//  1. If composite contains "@", look up the short hash; unknown → error.
//  2. Otherwise, if exactly one account is registered, default to it.
//  3. Otherwise (no suffix + multi-account), return ErrAmbiguousAccount.
//
// The explicit account_id path is handled separately in api_handlers.go.
func resolveOutgoingTarget(reg *accountRegistry, composite string) (*accountRuntime, string, error) {
	rawID, shortHash := decodeConversationID(composite)
	if rawID == "" {
		return nil, "", fmt.Errorf("channelConversationId is required")
	}
	if shortHash != "" {
		rt, ok := reg.GetByShortHash(shortHash)
		if !ok {
			return nil, "", fmt.Errorf("%w: shortHash=%s", ErrUnknownAccount, shortHash)
		}
		return rt, rawID, nil
	}
	if rt, ok := reg.Default(); ok {
		return rt, rawID, nil
	}
	return nil, "", ErrAmbiguousAccount
}

// resolveByAccountID is a shortcut used by admin/operational endpoints that
// accept an explicit "account_id" field.
func resolveByAccountID(reg *accountRegistry, accountID string) (*accountRuntime, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}
	rt, ok := reg.GetByID(accountID)
	if !ok {
		return nil, fmt.Errorf("%w: accountId=%s", ErrUnknownAccount, accountID)
	}
	return rt, nil
}
