## ADDED Requirements

### Requirement: Facade endpoint for group detail query

Channel-qiwei SHALL expose `POST /api/qiwei/get_group_detail` accepting a JSON body with `roomIds` (array of string). It SHALL call the platform API `/room/batchGetRoomDetail` with `{guid, roomIdList: roomIds}` and return a simplified response.

The response `data.groups` SHALL be an array of objects, each containing:
- `roomId` (string)
- `roomName` (string, base64-decoded if applicable)
- `announcement` (string)
- `createUserId` (string)
- `memberCount` (int, computed from memberList length)
- `members` (array of `{userId, nickname, isAdmin, joinTime}`)

#### Scenario: Query single group detail

- **WHEN** agent calls `POST /api/qiwei/get_group_detail` with `{"roomIds": ["10791082136095292"]}`
- **THEN** the response SHALL contain `{"success": true, "data": {"groups": [{"roomId": "10791082136095292", "roomName": "测试群", "announcement": "...", "createUserId": "168885...", "memberCount": 5, "members": [...]}]}}`

#### Scenario: Query non-existent group

- **WHEN** agent calls with a `roomIds` containing a non-existent ID
- **THEN** the response SHALL return `{"success": true, "data": {"groups": []}}` or the group SHALL be absent from the array

#### Scenario: Empty roomIds

- **WHEN** agent calls with empty `roomIds` array
- **THEN** the response SHALL return 400 with `{"success": false, "error": "roomIds is required"}`

---

### Requirement: Facade endpoint for contact detail query

Channel-qiwei SHALL expose `POST /api/qiwei/get_contact_detail` accepting a JSON body with `userIds` (array of string). It SHALL call the platform API `/contact/batchGetUserinfo` with `{guid, userIdList: userIds}` and return a simplified response.

The response `data.contacts` SHALL be an array of objects, each containing:
- `userId` (string)
- `nickname` (string)
- `realName` (string)
- `alias` (string)
- `corpId` (string)
- `gender` (int: 1=male, 2=female)
- `avatarUrl` (string)

#### Scenario: Query single contact detail

- **WHEN** agent calls `POST /api/qiwei/get_contact_detail` with `{"userIds": ["1688854961262919"]}`
- **THEN** the response SHALL contain `{"success": true, "data": {"contacts": [{"userId": "1688854961262919", "nickname": "张三", ...}]}}`

#### Scenario: Empty userIds

- **WHEN** agent calls with empty `userIds` array
- **THEN** the response SHALL return 400 with `{"success": false, "error": "userIds is required"}`

---

### Requirement: Agent tool wecom_get_group_detail

The agent SHALL register a builtin tool `wecom_get_group_detail` that calls `POST {qiweiBase}/api/qiwei/get_group_detail`.

Tool schema:
- `roomIds` (array of string, required): group IDs to query

The tool description SHALL explain in Chinese that this queries group details including name, announcement, and member list.

#### Scenario: Agent uses tool to query group detail

- **WHEN** LLM invokes `wecom_get_group_detail` with `{"roomIds": ["10791082136095292"]}`
- **THEN** the tool executor SHALL POST to `{qiweiBase}/api/qiwei/get_group_detail` with the same body and return the response

---

### Requirement: Agent tool wecom_get_contact_detail

The agent SHALL register a builtin tool `wecom_get_contact_detail` that calls `POST {qiweiBase}/api/qiwei/get_contact_detail`.

Tool schema:
- `userIds` (array of string, required): user IDs to query

The tool description SHALL explain in Chinese that this queries contact details including name, company, and avatar.

#### Scenario: Agent uses tool to query contact detail

- **WHEN** LLM invokes `wecom_get_contact_detail` with `{"userIds": ["1688854961262919"]}`
- **THEN** the tool executor SHALL POST to `{qiweiBase}/api/qiwei/get_contact_detail` with the same body and return the response
