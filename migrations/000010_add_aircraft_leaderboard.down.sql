DROP TABLE IF EXISTS cached_stats_json;

DELETE FROM user_settings WHERE setting_key = 'leaderboard_table_limit';
