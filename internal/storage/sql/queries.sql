-- name: CreateEvent :one
INSERT INTO events (
    id, type, payload, created_at, status, error, repository, sender
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetEvent :one
SELECT * FROM events WHERE id = $1;

-- name: ListEvents :many
SELECT * FROM events
WHERE
    ($1::varchar[] IS NULL OR type = ANY($1)) AND
    ($2::varchar IS NULL OR repository = $2) AND
    ($3::timestamp IS NULL OR created_at >= $3) AND
    ($4::timestamp IS NULL OR created_at <= $4) AND
    ($5::varchar IS NULL OR status = $5) AND
    ($6::varchar IS NULL OR sender = $6)
ORDER BY created_at DESC
LIMIT $7 OFFSET $8;

-- name: CountEvents :one
SELECT COUNT(*) FROM events
WHERE
    ($1::varchar[] IS NULL OR type = ANY($1)) AND
    ($2::varchar IS NULL OR repository = $2) AND
    ($3::timestamp IS NULL OR created_at >= $3) AND
    ($4::timestamp IS NULL OR created_at <= $4) AND
    ($5::varchar IS NULL OR status = $5) AND
    ($6::varchar IS NULL OR sender = $6);

-- name: UpdateEventStatus :one
UPDATE events
SET status = $2, error = $3
WHERE id = $1
RETURNING *;

-- name: DeleteEvent :exec
DELETE FROM events WHERE id = $1;

-- name: GetEventTypeStats :many
SELECT type, COUNT(*) as count
FROM events
WHERE
    ($1::timestamp IS NULL OR created_at >= $1) AND
    ($2::timestamp IS NULL OR created_at <= $2)
GROUP BY type
ORDER BY count DESC;
