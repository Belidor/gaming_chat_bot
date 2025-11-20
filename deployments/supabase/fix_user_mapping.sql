-- Fix User Mapping for Imported Messages
-- Replace old usernames from export with correct user data

-- =============================================================================
-- 1. Check current state
-- =============================================================================
SELECT 
    username,
    user_id,
    first_name,
    COUNT(*) as message_count
FROM chat_messages
WHERE username IN ('Artem Kupratsevich', 'Олег Пахомов', 'Рог')
GROUP BY username, user_id, first_name
ORDER BY username;

-- =============================================================================
-- 2. Update user mappings
-- =============================================================================

-- Update: Artem Kupratsevich -> Belidor
UPDATE chat_messages
SET 
    user_id = 65007397,
    username = 'Belidor',
    first_name = 'Artem'
WHERE username = 'Artem Kupratsevich';

-- Update: Олег Пахомов -> oleg_pax
UPDATE chat_messages
SET 
    user_id = 381179678,
    username = 'oleg_pax',
    first_name = 'Олег'
WHERE username = 'Олег Пахомов';

-- Update: Рог -> rogov_ps
UPDATE chat_messages
SET 
    user_id = 374262028,
    username = 'rogov_ps',
    first_name = 'Pavel'
WHERE username = 'Рог';

-- =============================================================================
-- 3. Verify the fix
-- =============================================================================
SELECT 
    user_id,
    username,
    first_name,
    COUNT(*) as message_count,
    COUNT(CASE WHEN indexed = TRUE THEN 1 END) as indexed_count
FROM chat_messages
GROUP BY user_id, username, first_name
ORDER BY user_id;

SELECT 'User mapping fix completed!' as status;

