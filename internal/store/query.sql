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
