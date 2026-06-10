package db

// Provisioning-template persistence. MOVED FROM db.go — db-layer split by
// domain (post-v0.5.2 review item 6); bodies unchanged.

func (db *DB) ListTemplateNames() ([]string, error) {
	rows, err := db.sql.Query(`SELECT name FROM templates ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (db *DB) ListTemplates() (map[string]string, error) {
	rows, err := db.sql.Query(`SELECT name, content FROM templates ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var name, content string
		if err := rows.Scan(&name, &content); err != nil {
			return nil, err
		}
		out[name] = content
	}
	return out, rows.Err()
}

func (db *DB) GetTemplate(name string) (string, string, error) {
	var content, credentialRef string
	err := db.sql.QueryRow(`SELECT content, credential_ref FROM templates WHERE name = ?`, name).Scan(&content, &credentialRef)
	return content, credentialRef, err
}

func (db *DB) SaveTemplate(name, content, credentialRef string) error {
	_, err := db.sql.Exec(`INSERT INTO templates(name, content, credential_ref, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET content=excluded.content, credential_ref=excluded.credential_ref`, name, content, credentialRef, now())
	return err
}

func (db *DB) DeleteTemplate(name string) error {
	_, err := db.sql.Exec(`DELETE FROM templates WHERE name = ?`, name)
	return err
}
