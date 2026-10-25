-- name: CreateInstance :exec
INSERT INTO instance (id, username, private_key_pem, public_key_pem)
VALUES (1, ?, ?, ?);

-- name: GetInstance :one
SELECT * FROM instance WHERE id = 1;
