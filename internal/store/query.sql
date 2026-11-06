-- name: CreateInstance :exec
INSERT INTO instance (id, username, private_key_pem, public_key_pem)
VALUES (1, ?, ?, ?);

-- name: GetInstance :one
SELECT * FROM instance WHERE id = 1;

-- name: BindInstanceHost :execrows
UPDATE instance SET host = ?, account_domain = ? WHERE id = 1 AND host IS NULL;

-- name: CreateProfile :exec
INSERT INTO profile (id, document) VALUES (1, ?);

-- name: GetProfile :one
SELECT document FROM profile WHERE id = 1;

-- name: UpsertFollower :exec
INSERT INTO followers (actor_iri, inbox, shared_inbox, follow_iri)
VALUES (?, ?, ?, ?)
ON CONFLICT (actor_iri) DO UPDATE SET
    inbox = excluded.inbox,
    shared_inbox = excluded.shared_inbox,
    follow_iri = excluded.follow_iri;

-- name: GetFollower :one
SELECT * FROM followers WHERE actor_iri = ?;

-- name: DeleteFollower :execrows
DELETE FROM followers WHERE actor_iri = ?;

-- name: CountFollowers :one
SELECT count(*) FROM followers;

-- name: ListFollowerInboxes :many
SELECT inbox, shared_inbox FROM followers;

-- name: CreateNote :exec
INSERT INTO notes (id, content, published_at) VALUES (?, ?, ?);

-- name: GetNote :one
SELECT * FROM notes WHERE id = ?;

-- name: ListNotes :many
SELECT * FROM notes ORDER BY published_at DESC LIMIT ?;

-- name: CountNotes :one
SELECT count(*) FROM notes;

-- name: EnqueueDelivery :exec
INSERT INTO delivery_queue (inbox, activity, next_attempt_at) VALUES (?, ?, ?);

-- name: DueDeliveries :many
SELECT * FROM delivery_queue WHERE next_attempt_at <= ? ORDER BY next_attempt_at, id LIMIT ?;

-- name: RescheduleDelivery :exec
UPDATE delivery_queue SET attempts = ?, next_attempt_at = ?, last_error = ? WHERE id = ?;

-- name: DeleteDelivery :exec
DELETE FROM delivery_queue WHERE id = ?;

-- name: CountDeliveries :one
SELECT count(*) FROM delivery_queue;

-- name: UpdateProfile :exec
UPDATE profile SET document = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = 1;

-- name: SetOwnerEmail :execrows
UPDATE instance SET owner_email = ? WHERE id = 1;

-- name: CreateLoginLink :exec
INSERT INTO login_links (token_hash, created_at) VALUES (?, ?);

-- name: CountLoginLinksSince :one
SELECT count(*) FROM login_links WHERE created_at > ?;

-- name: DeleteLoginLinksBefore :exec
DELETE FROM login_links WHERE created_at <= ?;

-- name: LatestLoginLink :one
SELECT * FROM login_links ORDER BY id DESC LIMIT 1;

-- name: UseLoginLink :execrows
UPDATE login_links SET used_at = ? WHERE id = ? AND used_at IS NULL;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, expires_at) VALUES (?, ?);

-- name: GetSessionExpiry :one
SELECT expires_at FROM sessions WHERE token_hash = ? AND expires_at > ?;

-- name: RenewSession :exec
UPDATE sessions SET expires_at = ? WHERE token_hash = ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = ?;

-- name: DeleteAllSessions :exec
DELETE FROM sessions;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= ?;

-- name: ListPlugins :many
SELECT * FROM plugins ORDER BY id;

-- name: GetPlugin :one
SELECT * FROM plugins WHERE id = ?;

-- name: SavePlugin :exec
INSERT INTO plugins (id, config, status, checked_at, enabled_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET config = excluded.config, status = excluded.status, checked_at = excluded.checked_at;

-- name: SetPluginStatus :exec
UPDATE plugins SET status = ?, checked_at = ? WHERE id = ?;

-- name: DeletePlugin :exec
DELETE FROM plugins WHERE id = ?;
