-- name: GetAdminStats :one
SELECT (SELECT COUNT(*) FROM users WHERE provider <> 'ANON')::integer AS users,
       (SELECT COUNT(*) FROM users WHERE provider = 'ANON')::integer AS anonymous_users,
       (SELECT COUNT(*) FROM users WHERE role = 'ADMIN')::integer AS admins,
       (SELECT COUNT(*) FROM projects)::integer AS projects,
       (SELECT COUNT(*) FROM comments)::integer AS comments,
       (SELECT COUNT(*) FROM ratings)::integer AS ratings,
       (SELECT COUNT(*) FROM contact_messages WHERE NOT is_read)::integer AS unread_messages;
