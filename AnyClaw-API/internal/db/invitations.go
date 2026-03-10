package db

import (
	"database/sql"
	"fmt"
)

func (d *DB) CreateInvitation(inviterID int64, code string) error {
	_, err := d.Exec("INSERT INTO invitations (code, inviter_id) VALUES (?, ?)", code, inviterID)
	return err
}

func (d *DB) UseInvitation(code string, inviteeID int64) (inviterID int64, err error) {
	var inviter int64
	err = d.QueryRow("SELECT inviter_id FROM invitations WHERE code = ? AND invitee_id IS NULL", code).Scan(&inviter)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("invalid or used invitation code")
	}
	if err != nil {
		return 0, err
	}
	_, err = d.Exec("UPDATE invitations SET invitee_id = ? WHERE code = ?", inviteeID, code)
	if err != nil {
		return 0, err
	}
	_ = d.SetUserInviter(inviteeID, inviter)
	return inviter, nil
}
