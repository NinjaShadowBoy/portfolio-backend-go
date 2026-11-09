-- name: AddChatUsage :exec
-- Adds to the day's counters of one subject, creating the row on first use.
INSERT INTO chat_usage (day, subject, requests, input_tokens, output_tokens)
VALUES (sqlc.arg(day), sqlc.arg(subject), sqlc.arg(requests), sqlc.arg(input_tokens), sqlc.arg(output_tokens))
ON CONFLICT (day, subject) DO UPDATE SET
    requests = chat_usage.requests + EXCLUDED.requests,
    input_tokens = chat_usage.input_tokens + EXCLUDED.input_tokens,
    output_tokens = chat_usage.output_tokens + EXCLUDED.output_tokens;

-- name: GetChatUsage :one
SELECT requests, input_tokens, output_tokens
FROM chat_usage
WHERE day = sqlc.arg(day) AND subject = sqlc.arg(subject);

-- name: ListChatDailyUsage :many
-- The global totals of one day. The use case returns a row per calendar day
-- of the range, so missing days mean a zero-traffic day, not an error.
SELECT day, requests, input_tokens, output_tokens
FROM chat_usage
WHERE subject = '*'
  AND day >= sqlc.arg(first_day) AND day <= sqlc.arg(last_day)
ORDER BY day DESC;

-- name: ListChatSubjectUsage :many
-- Per-subject totals over the range, best first; the global row is excluded.
SELECT subject,
       SUM(requests) AS requests,
       SUM(input_tokens) AS input_tokens,
       SUM(output_tokens) AS output_tokens
FROM chat_usage
WHERE subject <> '*'
  AND day >= sqlc.arg(first_day) AND day <= sqlc.arg(last_day)
GROUP BY subject
ORDER BY requests DESC, subject
LIMIT sqlc.arg(limit_rows)::int;
