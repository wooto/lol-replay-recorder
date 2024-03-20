CREATE TABLE IF NOT EXISTS youtube_posts
(
    id          SERIAL PRIMARY KEY,
    channel_id  VARCHAR(255),
    title       VARCHAR(255),
    created_at  TIMESTAMP
);
