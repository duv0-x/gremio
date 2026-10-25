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
