-- Fix Chat ID Format for Telegram Supergroups
-- 
-- Problem: Telegram Desktop JSON exports use positive chat IDs (e.g. 1750074031)
-- while Telegram Bot API uses negative IDs for supergroups (e.g. -1001750074031)
--
-- Solution: Normalize all chat_ids to use the Telegram Bot API format (-100 + original_id)

-- =============================================================================
-- 1. Check current state
-- =============================================================================
SELECT 
    chat_id,
    COUNT(*) as message_count,
    MIN(created_at) as oldest,
    MAX(created_at) as newest,
    COUNT(CASE WHEN indexed = TRUE THEN 1 END) as indexed_count
FROM chat_messages
GROUP BY chat_id
ORDER BY chat_id;

-- =============================================================================
-- 2. Fix duplicate messages and normalize chat_id
-- =============================================================================

-- Step 1: Find and keep only the LATEST version of each message_id
-- (Keep messages with correct negative chat_id, or indexed ones, or newest ones)

-- Delete older duplicates, keeping the best version of each message
DELETE FROM chat_messages
WHERE id IN (
    SELECT id FROM (
        SELECT 
            id,
            message_id,
            ROW_NUMBER() OVER (
                PARTITION BY message_id 
                ORDER BY 
                    CASE WHEN chat_id < 0 THEN 0 ELSE 1 END,  -- Prefer negative chat_id
                    CASE WHEN indexed = TRUE THEN 0 ELSE 1 END,  -- Prefer indexed
                    created_at DESC,  -- Prefer newer
                    id DESC  -- Final tiebreaker
            ) as rn
        FROM chat_messages
    ) ranked
    WHERE rn > 1
);

-- Step 2: Now safely update ALL remaining messages to correct chat_id
UPDATE chat_messages
SET chat_id = -1001750074031;

-- =============================================================================
-- 3. Verify the fix
-- =============================================================================
SELECT 
    chat_id,
    COUNT(*) as message_count,
    COUNT(CASE WHEN indexed = TRUE THEN 1 END) as indexed_count,
    COUNT(CASE WHEN indexed = FALSE THEN 1 END) as unindexed_count
FROM chat_messages
GROUP BY chat_id
ORDER BY chat_id;

-- =============================================================================
-- 4. Check for duplicates (should be none due to UNIQUE constraint)
-- =============================================================================
SELECT 
    chat_id,
    message_id,
    COUNT(*) as duplicate_count
FROM chat_messages
GROUP BY chat_id, message_id
HAVING COUNT(*) > 1;

-- Expected result: No duplicates

SELECT 'Chat ID normalization completed!' as status;

