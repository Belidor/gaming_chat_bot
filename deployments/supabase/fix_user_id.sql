-- Fix User ID for Imported Messages
-- 
-- Problem: Messages imported from Telegram Desktop JSON have user_id = 0
-- because the import script didn't parse the from_id field correctly.
--
-- Solution: Try to match user_id from existing messages with the same username

-- =============================================================================
-- 1. Check current state
-- =============================================================================
SELECT 
    user_id,
    username,
    first_name,
    COUNT(*) as message_count
FROM chat_messages
GROUP BY user_id, username, first_name
ORDER BY user_id, username;

-- =============================================================================
-- 2. Find messages with user_id = 0 that can be matched
-- =============================================================================
SELECT 
    cm1.username,
    cm1.first_name,
    COUNT(DISTINCT cm1.id) as messages_with_zero_id,
    ARRAY_AGG(DISTINCT cm2.user_id) FILTER (WHERE cm2.user_id != 0) as known_user_ids
FROM chat_messages cm1
LEFT JOIN chat_messages cm2 ON cm1.username = cm2.username AND cm2.user_id != 0
WHERE cm1.user_id = 0 AND cm1.username IS NOT NULL AND cm1.username != ''
GROUP BY cm1.username, cm1.first_name
ORDER BY messages_with_zero_id DESC;

-- =============================================================================
-- 3. Update user_id based on matching username
-- =============================================================================

-- Strategy: For each username with user_id=0, find their real user_id from other messages
-- and update all their messages

-- Create a temporary mapping table
CREATE TEMP TABLE user_id_mapping AS
SELECT DISTINCT
    cm1.username,
    cm2.user_id
FROM chat_messages cm1
JOIN chat_messages cm2 ON cm1.username = cm2.username
WHERE cm1.user_id = 0 
  AND cm2.user_id != 0
  AND cm1.username IS NOT NULL 
  AND cm1.username != '';

-- Show the mapping
SELECT * FROM user_id_mapping ORDER BY username;

-- Update messages with correct user_id
UPDATE chat_messages cm
SET user_id = m.user_id
FROM user_id_mapping m
WHERE cm.username = m.username
  AND cm.user_id = 0;

-- Get diagnostics
SELECT 
    CASE WHEN user_id = 0 THEN 'Unknown User' ELSE 'Known User' END as user_type,
    COUNT(*) as message_count
FROM chat_messages
GROUP BY user_type
ORDER BY user_type;

-- =============================================================================
-- 4. For users that couldn't be matched (still have user_id = 0)
-- =============================================================================

-- Show remaining messages with user_id = 0
SELECT 
    username,
    first_name,
    COUNT(*) as message_count,
    MIN(created_at) as first_message,
    MAX(created_at) as last_message
FROM chat_messages
WHERE user_id = 0
GROUP BY username, first_name
ORDER BY message_count DESC;

-- Note: These users only exist in the imported history and never sent messages
-- after the bot was deployed. Their user_id will remain 0 until they send a new message.

-- =============================================================================
-- 5. Cleanup
-- =============================================================================
DROP TABLE IF EXISTS user_id_mapping;

SELECT 'User ID fix completed!' as status;

-- Summary
SELECT 
    COUNT(*) as total_messages,
    COUNT(CASE WHEN user_id = 0 THEN 1 END) as messages_with_unknown_user,
    COUNT(CASE WHEN user_id != 0 THEN 1 END) as messages_with_known_user,
    ROUND(100.0 * COUNT(CASE WHEN user_id != 0 THEN 1 END) / COUNT(*), 2) as percentage_with_known_user
FROM chat_messages;

