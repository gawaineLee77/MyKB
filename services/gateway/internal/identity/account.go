package identity

import (
	"context"
	"strings"
)

// BindLocalAccount is the identity-only counterpart to BindLocalPrincipal.
// SQL NULL preserves the existing positive-or-NULL tenant constraint.
func (r *Repository) BindLocalAccount(ctx context.Context, email, userID string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(email) == "" {
		return ErrInvalid
	}
	result, err := r.db.ExecContext(ctx, `UPDATE mindcreek.corporate_identities SET local_user_id=$2
        WHERE upstream_email=$1 AND status='active' AND (local_user_id IS NULL OR local_user_id=$2)`, strings.ToLower(strings.TrimSpace(email)), userID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrUnlinked
	}
	return nil
}

// MemberAlias accepts a trusted corporate email or a previously issued native
// alias. Ambiguous corporate addresses must not choose an arbitrary employee.
func (r *Repository) MemberAlias(ctx context.Context, email string) (string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT upstream_email FROM mindcreek.corporate_identities
        WHERE (lower(corporate_email)=$1 OR upstream_email=$1) AND status='active' AND local_user_id IS NOT NULL`, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var alias string
	n := 0
	for rows.Next() {
		if err := rows.Scan(&alias); err != nil {
			return "", err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if n != 1 {
		return "", ErrNotFound
	}
	return alias, nil
}
