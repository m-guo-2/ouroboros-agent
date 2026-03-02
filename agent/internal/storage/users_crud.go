package storage

import "database/sql"

// UserRecord mirrors a row in the users table.
type UserRecord struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	AvatarURL string `json:"avatarUrl"`
	CreatedAt string `json:"createdAt"`
}

// GetUserByID returns a user by primary key, or (nil, nil) when not found.
func GetUserByID(userID string) (*UserRecord, error) {
	var u UserRecord
	err := DB.QueryRow(
		`SELECT id, COALESCE(name,''), COALESCE(type,'human'), COALESCE(avatar_url,''), COALESCE(created_at,'')
		 FROM users WHERE id = ?`, userID,
	).Scan(&u.ID, &u.Name, &u.Type, &u.AvatarURL, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

// GetAllUsers returns every user ordered by creation time.
func GetAllUsers() ([]UserRecord, error) {
	rows, err := DB.Query(
		`SELECT id, COALESCE(name,''), COALESCE(type,'human'), COALESCE(avatar_url,''), COALESCE(created_at,'')
		 FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserRecord
	for rows.Next() {
		var u UserRecord
		if err := rows.Scan(&u.ID, &u.Name, &u.Type, &u.AvatarURL, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
