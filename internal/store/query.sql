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

-- name: CreateNote :execrows
-- Un id repetido no hace nada: publicar dos veces el mismo evento no duplica el Note.
INSERT INTO notes (id, content, published_at, gremio) VALUES (?, ?, ?, ?) ON CONFLICT (id) DO NOTHING;

-- name: DeleteNote :execrows
DELETE FROM notes WHERE id = ?;

-- name: ListEventNotes :many
SELECT * FROM notes WHERE gremio != '' ORDER BY published_at DESC, id LIMIT ?;

-- name: HasEventNotes :one
SELECT EXISTS (SELECT 1 FROM notes WHERE gremio != '');

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

-- name: AppendDojoEvent :exec
INSERT INTO dojo_events (hash, prev, type, at, challenge, body) VALUES (?, ?, ?, ?, ?, ?);

-- name: LastDojoEventHash :one
SELECT hash FROM dojo_events ORDER BY seq DESC LIMIT 1;

-- name: GetDojoEvent :one
SELECT * FROM dojo_events WHERE hash = ?;

-- name: ListDojoEvents :many
SELECT * FROM dojo_events ORDER BY seq;

-- name: ScrubDojoEvent :execrows
UPDATE dojo_events SET body = ? WHERE hash = ?;

-- name: CreateChallenge :exec
INSERT INTO challenges (id, draft, created_at, updated_at) VALUES (?, ?, ?, ?);

-- name: GetChallenge :one
SELECT * FROM challenges WHERE id = ? AND deleted_at IS NULL;

-- name: ListChallenges :many
SELECT * FROM challenges WHERE deleted_at IS NULL ORDER BY updated_at DESC;

-- name: SaveChallengeDraft :execrows
UPDATE challenges SET draft = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL;

-- name: DeleteChallenge :execrows
UPDATE challenges SET draft = NULL, deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL;

-- name: LatestChallengeVersion :one
SELECT * FROM challenge_versions WHERE challenge_id = ? ORDER BY version DESC LIMIT 1;

-- name: GetChallengeVersion :one
SELECT * FROM challenge_versions WHERE challenge_id = ? AND version = ?;

-- name: CreateChallengeVersion :exec
INSERT INTO challenge_versions (challenge_id, version, document, published_at) VALUES (?, ?, ?, ?);

-- name: CreateExecution :exec
INSERT INTO executions (id, challenge, challenge_version, snapshot, status, started_at, created_at, updated_at)
VALUES (?, ?, ?, ?, 'in_progress', ?, ?, ?);

-- name: GetExecution :one
SELECT * FROM executions WHERE id = ?;

-- name: ListExecutions :many
SELECT * FROM executions ORDER BY updated_at DESC;

-- name: ListPublishedExecutions :many
SELECT * FROM executions WHERE published_at IS NOT NULL AND retired_at IS NULL ORDER BY published_at DESC;

-- name: AbandonExecution :execrows
UPDATE executions SET status = 'abandoned', updated_at = ? WHERE id = ? AND status IN ('draft', 'in_progress');

-- name: SaveDelivery :execrows
UPDATE executions SET repo = ?, commit_sha = ?, demo = ?, report = ?, notes = ?, verification = NULL, updated_at = ?
WHERE id = ? AND status IN ('draft', 'in_progress');

-- name: SetVerification :execrows
UPDATE executions SET verification = ?, updated_at = ? WHERE id = ? AND status IN ('draft', 'in_progress');

-- name: CompleteExecution :execrows
UPDATE executions SET status = 'done', completed_at = ?, updated_at = ? WHERE id = ? AND status IN ('draft', 'in_progress');

-- name: PublishExecution :execrows
UPDATE executions SET published_at = ?, updated_at = ? WHERE id = ? AND published_at IS NULL AND status != 'abandoned';

-- name: SetExecutionHashes :exec
UPDATE executions SET started_hash = ?, completed_hash = ?, updated_at = ? WHERE id = ?;

-- name: RetireExecution :execrows
UPDATE executions SET retired_at = ?, updated_at = ? WHERE id = ? AND published_at IS NOT NULL AND retired_at IS NULL;
