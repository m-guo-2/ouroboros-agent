## ADDED Requirements

### Requirement: Channel auto-accepts friend requests

When channel-qiwei receives a friend request callback (cmd=15500, msgType=2357), it SHALL automatically call the platform API `/contact/agreeContact` with the `contactId` (as userId) and associated `corpId` extracted from `msgData`.

The `guid` parameter SHALL be obtained from the callback message or from channel config.

#### Scenario: Friend request from enterprise WeChat user

- **WHEN** a callback arrives with `cmd=15500`, `msgType=2357`, `msgData.contactId=78813061361`, and the callback `userId` field provides the corpId context
- **THEN** channel-qiwei SHALL call `/contact/agreeContact` with `{guid, userId: "78813061361", corpId: "<extracted>"}`

#### Scenario: Auto-accept API call fails

- **WHEN** the `/contact/agreeContact` API call returns an error or non-success code
- **THEN** channel-qiwei SHALL log the error at Business level and continue (not crash or retry)

---

### Requirement: Channel pushes new_contact event to agent after auto-accept

After successfully auto-accepting a friend request, channel-qiwei SHALL send a `GroupEvent` to the agent's `/api/channels/group-event` endpoint with:
- `eventType`: `"new_contact"`
- `channel`: `"qiwei"`
- `agentId`: from config
- `payload.contactId`: the new contact's userId (string)
- `payload.contactNickname`: the new contact's nickname (string, from callback `msgData.contactNickname`)
- `payload.contactType`: the contact type (string, from callback `msgData.contactType`, e.g. "微信")

#### Scenario: Successful auto-accept triggers new_contact event

- **WHEN** `/contact/agreeContact` succeeds for contactId `"78813061361"` with nickname `"nihao～"` and contactType `"微信"`
- **THEN** channel-qiwei SHALL POST to agent `/api/channels/group-event` with payload `{"channel":"qiwei","agentId":"...","channelGroupId":"","eventType":"new_contact","payload":{"contactId":"78813061361","contactNickname":"nihao～","contactType":"微信"}}`

#### Scenario: Auto-accept failed, no event pushed

- **WHEN** `/contact/agreeContact` fails
- **THEN** channel-qiwei SHALL NOT push a `new_contact` event to agent
