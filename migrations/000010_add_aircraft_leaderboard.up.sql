-- Create table for cached JSON statistics, e.g. the "Most Seen Aircraft" leaderboard.
-- Mirrors cached_stats, but stores a JSON payload instead of a single value.
CREATE TABLE cached_stats_json (
    stat_key VARCHAR(50) PRIMARY KEY,
    stat_value JSONB NOT NULL,
    last_updated TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add user setting for the number of rows to display in the "Most Seen Aircraft" leaderboard
INSERT INTO user_settings (setting_key, setting_value, description) VALUES
    ('leaderboard_table_limit', '10', 'Number of rows to display in the Most Seen Aircraft leaderboard');
