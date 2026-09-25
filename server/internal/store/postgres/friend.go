package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/friend"
)

// FriendRepository implements friend.Repository against PostgreSQL.
//
// Every write that depends on the relationship state between two users
// (sending/accepting a request, blocking) runs in a transaction holding a
// per-pair advisory lock, so its checks (is either user blocked? already
// friends? request already pending?) and its write are atomic with respect
// to every other relationship write for the same pair. Without it, a Block
// racing an AcceptRequest could leave a friendship between users where one
// has blocked the other.
type FriendRepository struct {
	pool *pgxpool.Pool
}

func NewFriendRepository(pool *pgxpool.Pool) *FriendRepository {
	return &FriendRepository{pool: pool}
}

const friendRequestColumns = `
	fr.id, fr.sender_id, su.username, fr.recipient_id, ru.username,
	fr.status, fr.created_at, fr.responded_at`

const friendRequestFrom = `
	FROM friend_requests fr
	JOIN users su ON su.id = fr.sender_id
	JOIN users ru ON ru.id = fr.recipient_id`

func scanFriendRequest(row pgx.Row) (*friend.FriendRequest, error) {
	var req friend.FriendRequest
	var status string
	if err := row.Scan(&req.ID, &req.SenderID, &req.SenderUsername, &req.RecipientID, &req.RecipientUsername,
		&status, &req.CreatedAt, &req.RespondedAt); err != nil {
		return nil, err
	}
	req.Status = friend.RequestStatus(status)
	return &req, nil
}

func getFriendRequest(ctx context.Context, q pgx.Tx, requestID string) (*friend.FriendRequest, error) {
	req, err := scanFriendRequest(q.QueryRow(ctx, `SELECT`+friendRequestColumns+friendRequestFrom+` WHERE fr.id = $1`, requestID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
			return nil, friend.ErrNotFound
		}
		return nil, err
	}
	return req, nil
}

// lockPair serializes relationship writes for one unordered pair of users
// for the rest of tx.
func lockPair(ctx context.Context, tx pgx.Tx, userID1, userID2 string) error {
	_, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended(
			least($1::text, $2::text) || ':' || greatest($1::text, $2::text), 0))
	`, userID1, userID2)
	return err
}

// eitherBlocked reports whether either user has blocked the other.
func eitherBlocked(ctx context.Context, tx pgx.Tx, userID1, userID2 string) (bool, error) {
	var blocked bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM blocks
			WHERE (blocker_id = $1 AND blocked_id = $2) OR (blocker_id = $2 AND blocked_id = $1)
		)
	`, userID1, userID2).Scan(&blocked)
	return blocked, err
}

// userExists reports whether userID names an existing user; a malformed ID
// is simply "not found".
func userExists(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists)
	if err != nil {
		if isInvalidUUID(err) {
			return false, nil
		}
		return false, err
	}
	return exists, nil
}

func (r *FriendRepository) ResolveUser(ctx context.Context, identifier string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM users WHERE lower(username) = lower($1) OR lower(email) = lower($1)
	`, identifier).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", friend.ErrNotFound
		}
		return "", err
	}
	return id, nil
}

func (r *FriendRepository) SendRequest(ctx context.Context, senderID, recipientID string) (*friend.FriendRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	for _, id := range []string{senderID, recipientID} {
		exists, err := userExists(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, friend.ErrNotFound
		}
	}
	if err := lockPair(ctx, tx, senderID, recipientID); err != nil {
		return nil, err
	}

	var blocked, friends, pending bool
	err = tx.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM blocks
			        WHERE (blocker_id = $1 AND blocked_id = $2) OR (blocker_id = $2 AND blocked_id = $1)),
			EXISTS (SELECT 1 FROM friendships
			        WHERE user_a_id = LEAST($1::uuid, $2::uuid) AND user_b_id = GREATEST($1::uuid, $2::uuid)),
			EXISTS (SELECT 1 FROM friend_requests
			        WHERE status = 'pending'
			          AND ((sender_id = $1 AND recipient_id = $2) OR (sender_id = $2 AND recipient_id = $1)))
	`, senderID, recipientID).Scan(&blocked, &friends, &pending)
	if err != nil {
		return nil, err
	}
	switch {
	case blocked:
		return nil, friend.ErrBlocked
	case friends:
		return nil, friend.ErrAlreadyFriends
	case pending:
		return nil, friend.ErrRequestPending
	}

	// (sender_id, recipient_id) is unique: a previously accepted or declined
	// request for this pair is re-opened rather than duplicated.
	var requestID string
	err = tx.QueryRow(ctx, `
		INSERT INTO friend_requests (sender_id, recipient_id, status)
		VALUES ($1, $2, 'pending')
		ON CONFLICT (sender_id, recipient_id) DO UPDATE
			SET status = 'pending', created_at = now(), responded_at = NULL
		RETURNING id
	`, senderID, recipientID).Scan(&requestID)
	if err != nil {
		return nil, err
	}

	req, err := getFriendRequest(ctx, tx, requestID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return req, nil
}

func (r *FriendRepository) GetRequest(ctx context.Context, requestID string) (*friend.FriendRequest, error) {
	req, err := scanFriendRequest(r.pool.QueryRow(ctx, `SELECT`+friendRequestColumns+friendRequestFrom+` WHERE fr.id = $1`, requestID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
			return nil, friend.ErrNotFound
		}
		return nil, err
	}
	return req, nil
}

func (r *FriendRepository) listRequests(ctx context.Context, where, userID string) ([]friend.FriendRequest, error) {
	rows, err := r.pool.Query(ctx, `SELECT`+friendRequestColumns+friendRequestFrom+`
		WHERE `+where+` AND fr.status = 'pending'
		ORDER BY fr.created_at DESC, fr.id DESC`, userID)
	if err != nil {
		if isInvalidUUID(err) {
			return []friend.FriendRequest{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	out := []friend.FriendRequest{}
	for rows.Next() {
		req, err := scanFriendRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *req)
	}
	return out, rows.Err()
}

func (r *FriendRepository) ListIncomingRequests(ctx context.Context, userID string) ([]friend.FriendRequest, error) {
	return r.listRequests(ctx, "fr.recipient_id = $1", userID)
}

func (r *FriendRepository) ListOutgoingRequests(ctx context.Context, userID string) ([]friend.FriendRequest, error) {
	return r.listRequests(ctx, "fr.sender_id = $1", userID)
}

func (r *FriendRepository) AcceptRequest(ctx context.Context, requestID string) (*friend.FriendRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	req, err := getFriendRequest(ctx, tx, requestID)
	if err != nil {
		return nil, err
	}
	if err := lockPair(ctx, tx, req.SenderID, req.RecipientID); err != nil {
		return nil, err
	}

	// Blocks are re-checked at accept time (docs/ARCHITECTURE.md: "enforce
	// blocking rules before allowing a Friend Request to be created or
	// accepted"); Block also deletes pending requests, so this only catches
	// a block that raced this accept.
	blocked, err := eitherBlocked(ctx, tx, req.SenderID, req.RecipientID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, friend.ErrBlocked
	}

	tag, err := tx.Exec(ctx, `
		UPDATE friend_requests SET status = 'accepted', responded_at = now()
		WHERE id = $1 AND status = 'pending'
	`, requestID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, friend.ErrNotPending
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO friendships (user_a_id, user_b_id)
		VALUES (LEAST($1::uuid, $2::uuid), GREATEST($1::uuid, $2::uuid))
		ON CONFLICT (user_a_id, user_b_id) DO NOTHING
	`, req.SenderID, req.RecipientID); err != nil {
		return nil, err
	}

	updated, err := getFriendRequest(ctx, tx, requestID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *FriendRepository) DeclineRequest(ctx context.Context, requestID string) (*friend.FriendRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE friend_requests SET status = 'declined', responded_at = now()
		WHERE id = $1 AND status = 'pending'
	`, requestID)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, friend.ErrNotFound
		}
		return nil, err
	}
	req, err := getFriendRequest(ctx, tx, requestID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, friend.ErrNotPending
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return req, nil
}

func (r *FriendRepository) CancelRequest(ctx context.Context, requestID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM friend_requests WHERE id = $1 AND status = 'pending'`, requestID)
	if err != nil {
		if isInvalidUUID(err) {
			return friend.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetRequest(ctx, requestID); err != nil {
			return err
		}
		return friend.ErrNotPending
	}
	return nil
}

func (r *FriendRepository) AreFriends(ctx context.Context, userID1, userID2 string) (bool, error) {
	return areFriends(ctx, r.pool, userID1, userID2)
}

// areFriends is shared with ProfileRepository.AreFollowers so both agree on
// what "friends" means.
func areFriends(ctx context.Context, pool *pgxpool.Pool, userID1, userID2 string) (bool, error) {
	var friends bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM friendships
			WHERE (user_a_id = $1 AND user_b_id = $2) OR (user_a_id = $2 AND user_b_id = $1)
		)
	`, userID1, userID2).Scan(&friends)
	if err != nil {
		if isInvalidUUID(err) {
			return false, nil
		}
		return false, err
	}
	return friends, nil
}

func (r *FriendRepository) ListFriends(ctx context.Context, userID string) ([]friend.Friend, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.username, f.since
		FROM friendships f
		JOIN users u ON u.id = CASE WHEN f.user_a_id = $1 THEN f.user_b_id ELSE f.user_a_id END
		WHERE f.user_a_id = $1 OR f.user_b_id = $1
		ORDER BY lower(u.username), u.id
	`, userID)
	if err != nil {
		if isInvalidUUID(err) {
			return []friend.Friend{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	out := []friend.Friend{}
	for rows.Next() {
		var f friend.Friend
		if err := rows.Scan(&f.UserID, &f.Username, &f.Since); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *FriendRepository) RemoveFriendship(ctx context.Context, userID1, userID2 string) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM friendships
		WHERE (user_a_id = $1 AND user_b_id = $2) OR (user_a_id = $2 AND user_b_id = $1)
	`, userID1, userID2)
	if err != nil {
		if isInvalidUUID(err) {
			return friend.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return friend.ErrNotFound
	}
	return nil
}

func (r *FriendRepository) Block(ctx context.Context, blockerID, blockedID string) (*friend.Block, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	exists, err := userExists(ctx, tx, blockedID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, friend.ErrNotFound
	}
	if err := lockPair(ctx, tx, blockerID, blockedID); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)
		ON CONFLICT (blocker_id, blocked_id) DO NOTHING
	`, blockerID, blockedID); err != nil {
		if isForeignKeyViolation(err) {
			return nil, friend.ErrNotFound
		}
		return nil, err
	}

	// docs/PRD.md § Blocking: any existing friendship is removed and Friend
	// Requests between the two users are disabled.
	if _, err := tx.Exec(ctx, `
		DELETE FROM friendships
		WHERE (user_a_id = $1 AND user_b_id = $2) OR (user_a_id = $2 AND user_b_id = $1)
	`, blockerID, blockedID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM friend_requests
		WHERE status = 'pending'
		  AND ((sender_id = $1 AND recipient_id = $2) OR (sender_id = $2 AND recipient_id = $1))
	`, blockerID, blockedID); err != nil {
		return nil, err
	}

	var b friend.Block
	if err := tx.QueryRow(ctx, `
		SELECT b.id, b.blocker_id, b.blocked_id, u.username, b.since
		FROM blocks b JOIN users u ON u.id = b.blocked_id
		WHERE b.blocker_id = $1 AND b.blocked_id = $2
	`, blockerID, blockedID).Scan(&b.ID, &b.BlockerID, &b.BlockedID, &b.BlockedUsername, &b.Since); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *FriendRepository) Unblock(ctx context.Context, blockerID, blockedID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2`, blockerID, blockedID)
	if err != nil {
		if isInvalidUUID(err) {
			return friend.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return friend.ErrNotFound
	}
	return nil
}

func (r *FriendRepository) IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error) {
	return isBlocked(ctx, r.pool, blockerID, blockedID)
}

// isBlocked is shared with ProfileRepository.IsBlocked.
func isBlocked(ctx context.Context, pool *pgxpool.Pool, blockerID, blockedID string) (bool, error) {
	var blocked bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM blocks WHERE blocker_id = $1 AND blocked_id = $2)
	`, blockerID, blockedID).Scan(&blocked)
	if err != nil {
		if isInvalidUUID(err) {
			return false, nil
		}
		return false, err
	}
	return blocked, nil
}

func (r *FriendRepository) ListBlocked(ctx context.Context, blockerID string) ([]friend.Block, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT b.id, b.blocker_id, b.blocked_id, u.username, b.since
		FROM blocks b JOIN users u ON u.id = b.blocked_id
		WHERE b.blocker_id = $1
		ORDER BY lower(u.username), u.id
	`, blockerID)
	if err != nil {
		if isInvalidUUID(err) {
			return []friend.Block{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	out := []friend.Block{}
	for rows.Next() {
		var b friend.Block
		if err := rows.Scan(&b.ID, &b.BlockerID, &b.BlockedID, &b.BlockedUsername, &b.Since); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

var _ friend.Repository = (*FriendRepository)(nil)
