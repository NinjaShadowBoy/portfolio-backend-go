INSERT INTO users (email, pwd, name, role, provider, provider_id, image_url, email_verified, last_login_at, created_at, updated_at)
SELECT email, pwd, name, role, provider, provider_id, image_url, email_verified, last_login_at, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM (VALUES
    ('alex@example.com', 'password123', 'Alex Abena', 'ADMIN', 'LOCAL', 'local-alex', 'https://images.unsplash.com/photo-1500648767791-00dcc994a43e?auto=format&fit=crop&w=400&q=80', TRUE, CURRENT_TIMESTAMP),
    ('demo@example.com', 'password123', 'Demo User', 'USER', 'LOCAL', 'local-demo', NULL, TRUE, CURRENT_TIMESTAMP)
) AS seed_users(email, pwd, name, role, provider, provider_id, image_url, email_verified, last_login_at)
WHERE NOT EXISTS (
    SELECT 1
    FROM users u
    WHERE u.email = seed_users.email
);

UPDATE users
SET name = 'Alex Abena',
    role = 'ADMIN',
    provider = 'LOCAL',
    provider_id = 'local-alex',
    image_url = 'https://images.unsplash.com/photo-1500648767791-00dcc994a43e?auto=format&fit=crop&w=400&q=80',
    email_verified = TRUE,
    last_login_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE email = 'alex@example.com';

UPDATE users
SET name = 'Demo User',
    role = 'USER',
    provider = 'LOCAL',
    provider_id = 'local-demo',
    image_url = NULL,
    email_verified = TRUE,
    last_login_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE email = 'demo@example.com';

INSERT INTO projects (name, description, featured, created_at, updated_at)
SELECT name, description, featured, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM (VALUES
    ('Portfolio Platform', 'A fast portfolio experience with project showcases, admin tools, contact form handling, and OAuth-ready authentication.', TRUE),
    ('Cloud Gallery API', 'A backend service for managing cloud-hosted images, project metadata, and public portfolio content.', TRUE),
    ('Analytics Dashboard', 'A responsive dashboard concept for tracking engagement, ratings, and portfolio maintenance tasks.', FALSE)
) AS seed_projects(name, description, featured)
WHERE NOT EXISTS (
    SELECT 1
    FROM projects p
    WHERE p.name = seed_projects.name
);

WITH project AS (
    SELECT id
    FROM projects
    WHERE name = 'Portfolio Platform'
), seed_technologies(technology) AS (
    VALUES
        ('Angular'),
        ('Go'),
        ('PostgreSQL'),
        ('JWT')
)
INSERT INTO project_technologies (project_id, technology)
SELECT project.id, seed_technologies.technology
FROM project
CROSS JOIN seed_technologies
WHERE NOT EXISTS (
    SELECT 1
    FROM project_technologies pt
    WHERE pt.project_id = project.id
      AND pt.technology = seed_technologies.technology
);

WITH project AS (
    SELECT id
    FROM projects
    WHERE name = 'Cloud Gallery API'
), seed_technologies(technology) AS (
    VALUES
        ('Spring Boot'),
        ('Kotlin'),
        ('Cloudinary'),
        ('OAuth2')
)
INSERT INTO project_technologies (project_id, technology)
SELECT project.id, seed_technologies.technology
FROM project
CROSS JOIN seed_technologies
WHERE NOT EXISTS (
    SELECT 1
    FROM project_technologies pt
    WHERE pt.project_id = project.id
      AND pt.technology = seed_technologies.technology
);

WITH project AS (
    SELECT id
    FROM projects
    WHERE name = 'Analytics Dashboard'
), seed_technologies(technology) AS (
    VALUES
        ('React'),
        ('TypeScript'),
        ('Charts')
)
INSERT INTO project_technologies (project_id, technology)
SELECT project.id, seed_technologies.technology
FROM project
CROSS JOIN seed_technologies
WHERE NOT EXISTS (
    SELECT 1
    FROM project_technologies pt
    WHERE pt.project_id = project.id
      AND pt.technology = seed_technologies.technology
);

WITH project AS (
    SELECT id
    FROM projects
    WHERE name = 'Portfolio Platform'
), seed_photos(photo_url) AS (
    VALUES
        ('https://images.unsplash.com/photo-1498050108023-c5249f4df085?auto=format&fit=crop&w=1200&q=80'),
        ('https://images.unsplash.com/photo-1460925895917-afdab827c52f?auto=format&fit=crop&w=1200&q=80')
)
INSERT INTO photos (photo_url, project_id, created_at, updated_at)
SELECT seed_photos.photo_url, project.id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM project
CROSS JOIN seed_photos
WHERE NOT EXISTS (
    SELECT 1
    FROM photos ph
    WHERE ph.project_id = project.id
      AND ph.photo_url = seed_photos.photo_url
);

WITH project AS (
    SELECT id
    FROM projects
    WHERE name = 'Cloud Gallery API'
), seed_photos(photo_url) AS (
    VALUES
        ('https://images.unsplash.com/photo-1558494949-ef010cbdcc31?auto=format&fit=crop&w=1200&q=80'),
        ('https://images.unsplash.com/photo-1551288049-bebda4e38f71?auto=format&fit=crop&w=1200&q=80')
)
INSERT INTO photos (photo_url, project_id, created_at, updated_at)
SELECT seed_photos.photo_url, project.id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM project
CROSS JOIN seed_photos
WHERE NOT EXISTS (
    SELECT 1
    FROM photos ph
    WHERE ph.project_id = project.id
      AND ph.photo_url = seed_photos.photo_url
);

WITH project AS (
    SELECT id
    FROM projects
    WHERE name = 'Analytics Dashboard'
), seed_photos(photo_url) AS (
    VALUES
        ('https://images.unsplash.com/photo-1551434678-e076c223a692?auto=format&fit=crop&w=1200&q=80')
)
INSERT INTO photos (photo_url, project_id, created_at, updated_at)
SELECT seed_photos.photo_url, project.id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM project
CROSS JOIN seed_photos
WHERE NOT EXISTS (
    SELECT 1
    FROM photos ph
    WHERE ph.project_id = project.id
      AND ph.photo_url = seed_photos.photo_url
);

WITH project AS (
    SELECT id
    FROM projects
    WHERE name = 'Portfolio Platform'
), seed_user AS (
    SELECT id
    FROM users
    WHERE email = 'alex@example.com'
), seed_rating(rating) AS (
    VALUES (5)
)
INSERT INTO ratings (rating, user_id, project_id, created_at, updated_at)
SELECT seed_rating.rating, seed_user.id, project.id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM seed_rating
CROSS JOIN seed_user
CROSS JOIN project
WHERE NOT EXISTS (
    SELECT 1
    FROM ratings r
    WHERE r.user_id = seed_user.id
      AND r.project_id = project.id
);

WITH project AS (
    SELECT id
    FROM projects
    WHERE name = 'Portfolio Platform'
), seed_user AS (
    SELECT id
    FROM users
    WHERE email = 'alex@example.com'
), seed_comment(body) AS (
    VALUES ('Fast, polished, and easy to navigate.')
)
INSERT INTO comments (body, user_id, project_id, created_at, updated_at)
SELECT seed_comment.body, seed_user.id, project.id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM seed_comment
CROSS JOIN seed_user
CROSS JOIN project
WHERE NOT EXISTS (
    SELECT 1
    FROM comments c
    WHERE c.user_id = seed_user.id
      AND c.project_id = project.id
      AND c.body = seed_comment.body
);

WITH project AS (
    SELECT id
    FROM projects
    WHERE name = 'Cloud Gallery API'
), seed_user AS (
    SELECT id
    FROM users
    WHERE email = 'demo@example.com'
), seed_rating(rating) AS (
    VALUES (4)
)
INSERT INTO ratings (rating, user_id, project_id, created_at, updated_at)
SELECT seed_rating.rating, seed_user.id, project.id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM seed_rating
CROSS JOIN seed_user
CROSS JOIN project
WHERE NOT EXISTS (
    SELECT 1
    FROM ratings r
    WHERE r.user_id = seed_user.id
      AND r.project_id = project.id
);

WITH project AS (
    SELECT id
    FROM projects
    WHERE name = 'Cloud Gallery API'
), seed_user AS (
    SELECT id
    FROM users
    WHERE email = 'demo@example.com'
), seed_comment(body) AS (
    VALUES ('Clean API design with useful admin controls.')
)
INSERT INTO comments (body, user_id, project_id, created_at, updated_at)
SELECT seed_comment.body, seed_user.id, project.id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM seed_comment
CROSS JOIN seed_user
CROSS JOIN project
WHERE NOT EXISTS (
    SELECT 1
    FROM comments c
    WHERE c.user_id = seed_user.id
      AND c.project_id = project.id
      AND c.body = seed_comment.body
);
