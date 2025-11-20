-- RAG System Migration for Telegram LLM Bot
-- Database: Supabase PostgreSQL
-- Purpose: Enable RAG search over entire chat history

-- =============================================================================
-- 1. Enable pgvector extension
-- =============================================================================
CREATE EXTENSION IF NOT EXISTS vector;

COMMENT ON EXTENSION vector IS 'pgvector extension for storing and querying vector embeddings';

-- =============================================================================
-- 2. Chat Messages Table (ALL messages from group chat)
-- =============================================================================
CREATE TABLE IF NOT EXISTS chat_messages (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL,                 -- Telegram Message ID
    user_id BIGINT NOT NULL,                    -- Telegram User ID
    username TEXT,                              -- Telegram Username (without @)
    first_name TEXT,                            -- Telegram First Name
    chat_id BIGINT NOT NULL,                    -- Telegram Chat ID
    message_text TEXT NOT NULL,                 -- Full message text
    embedding VECTOR(768),                      -- Gemini text-embedding-004 (768 dimensions)
    indexed BOOLEAN DEFAULT FALSE,              -- Whether embedding has been generated
    created_at TIMESTAMPTZ NOT NULL,            -- Message timestamp (from Telegram)
    indexed_at TIMESTAMPTZ,                     -- When embedding was generated
    
    CONSTRAINT unique_message UNIQUE(chat_id, message_id)
);

-- =============================================================================
-- 3. Indexes for Performance
-- =============================================================================

-- Index for user queries
CREATE INDEX IF NOT EXISTS idx_chat_messages_user_id 
    ON chat_messages(user_id);

-- Index for time-based queries
CREATE INDEX IF NOT EXISTS idx_chat_messages_created_at 
    ON chat_messages(created_at DESC);

-- Index for chat-specific queries
CREATE INDEX IF NOT EXISTS idx_chat_messages_chat_id 
    ON chat_messages(chat_id);

-- Composite index for unindexed messages (used by sync job)
CREATE INDEX IF NOT EXISTS idx_chat_messages_unindexed 
    ON chat_messages(indexed, created_at DESC) 
    WHERE indexed = FALSE;

-- Vector similarity index using IVFFlat algorithm (cosine distance)
-- Note: This index should be created AFTER inserting some data (at least 1000 rows)
-- For now, we'll create it with lists=100, can be optimized later
CREATE INDEX IF NOT EXISTS idx_chat_messages_embedding 
    ON chat_messages 
    USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

-- =============================================================================
-- 4. PostgreSQL Functions for RAG
-- =============================================================================

-- Function: Search similar messages using vector similarity
CREATE OR REPLACE FUNCTION search_similar_messages(
    query_embedding VECTOR(768),
    similarity_threshold FLOAT DEFAULT 0.8,
    match_count INT DEFAULT 5,
    target_chat_id BIGINT DEFAULT NULL
)
RETURNS TABLE (
    id BIGINT,
    message_id BIGINT,
    user_id BIGINT,
    username TEXT,
    first_name TEXT,
    chat_id BIGINT,
    message_text TEXT,
    created_at TIMESTAMPTZ,
    similarity FLOAT
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        cm.id,
        cm.message_id,
        cm.user_id,
        cm.username,
        cm.first_name,
        cm.chat_id,
        cm.message_text,
        cm.created_at,
        1 - (cm.embedding <=> query_embedding) as similarity
    FROM chat_messages cm
    WHERE 
        cm.indexed = TRUE 
        AND cm.embedding IS NOT NULL
        AND (target_chat_id IS NULL OR cm.chat_id = target_chat_id)
        AND (1 - (cm.embedding <=> query_embedding)) >= similarity_threshold
    ORDER BY cm.embedding <=> query_embedding
    LIMIT match_count;
END;
$$ LANGUAGE plpgsql;

-- Function: Get unindexed messages (for sync job)
CREATE OR REPLACE FUNCTION get_unindexed_messages(
    batch_size INT DEFAULT 100
)
RETURNS TABLE (
    id BIGINT,
    message_id BIGINT,
    user_id BIGINT,
    username TEXT,
    first_name TEXT,
    chat_id BIGINT,
    message_text TEXT,
    created_at TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        cm.id,
        cm.message_id,
        cm.user_id,
        cm.username,
        cm.first_name,
        cm.chat_id,
        cm.message_text,
        cm.created_at
    FROM chat_messages cm
    WHERE cm.indexed = FALSE
    ORDER BY cm.created_at ASC
    LIMIT batch_size;
END;
$$ LANGUAGE plpgsql;

-- Function: Update message embedding (atomic operation)
CREATE OR REPLACE FUNCTION update_message_embedding(
    p_message_id BIGINT,
    p_embedding VECTOR(768)
)
RETURNS BOOLEAN AS $$
DECLARE
    rows_updated INT;
BEGIN
    UPDATE chat_messages
    SET 
        embedding = p_embedding,
        indexed = TRUE,
        indexed_at = NOW()
    WHERE id = p_message_id;
    
    GET DIAGNOSTICS rows_updated = ROW_COUNT;
    
    RETURN rows_updated > 0;
END;
$$ LANGUAGE plpgsql;

-- Function: Batch update embeddings (for efficiency)
CREATE OR REPLACE FUNCTION batch_update_embeddings(
    p_message_ids BIGINT[],
    p_embeddings VECTOR(768)[]
)
RETURNS TABLE(rows_updated INT) AS $$
DECLARE
    updated_count INT := 0;
    i INT;
BEGIN
    -- Validate input arrays have same length
    IF array_length(p_message_ids, 1) != array_length(p_embeddings, 1) THEN
        RAISE EXCEPTION 'Message IDs and embeddings arrays must have same length';
    END IF;
    
    -- Update each message
    FOR i IN 1..array_length(p_message_ids, 1) LOOP
        UPDATE chat_messages
        SET 
            embedding = p_embeddings[i],
            indexed = TRUE,
            indexed_at = NOW()
        WHERE id = p_message_ids[i];
        
        IF FOUND THEN
            updated_count := updated_count + 1;
        END IF;
    END LOOP;
    
    RETURN QUERY SELECT updated_count;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- 5. Views for Analytics
-- =============================================================================

-- View: RAG indexing statistics
CREATE OR REPLACE VIEW rag_statistics AS
SELECT 
    COUNT(*) as total_messages,
    COUNT(CASE WHEN indexed = TRUE THEN 1 END) as indexed_messages,
    COUNT(CASE WHEN indexed = FALSE THEN 1 END) as unindexed_messages,
    ROUND(
        100.0 * COUNT(CASE WHEN indexed = TRUE THEN 1 END) / NULLIF(COUNT(*), 0), 
        2
    ) as indexed_percentage,
    MIN(created_at) as oldest_message,
    MAX(created_at) as newest_message,
    MAX(indexed_at) as last_indexing
FROM chat_messages;

COMMENT ON VIEW rag_statistics IS 'Overview of RAG indexing status';

-- View: Daily message statistics
CREATE OR REPLACE VIEW daily_message_stats AS
SELECT 
    DATE(created_at AT TIME ZONE 'Europe/Moscow') as date,
    COUNT(*) as total_messages,
    COUNT(DISTINCT user_id) as unique_users,
    COUNT(CASE WHEN indexed = TRUE THEN 1 END) as indexed_count,
    AVG(LENGTH(message_text)) as avg_message_length
FROM chat_messages
GROUP BY DATE(created_at AT TIME ZONE 'Europe/Moscow')
ORDER BY date DESC;

COMMENT ON VIEW daily_message_stats IS 'Daily statistics for chat messages';

-- =============================================================================
-- 6. Comments for Documentation
-- =============================================================================

COMMENT ON TABLE chat_messages IS 'Stores ALL messages from group chat for RAG search';

COMMENT ON COLUMN chat_messages.message_id IS 'Original Telegram message ID';
COMMENT ON COLUMN chat_messages.message_text IS 'Full text of the message';
COMMENT ON COLUMN chat_messages.embedding IS 'Vector embedding from Gemini text-embedding-004 (768 dimensions)';
COMMENT ON COLUMN chat_messages.indexed IS 'TRUE if embedding has been generated, FALSE if pending';
COMMENT ON COLUMN chat_messages.created_at IS 'Original timestamp from Telegram message';
COMMENT ON COLUMN chat_messages.indexed_at IS 'Timestamp when embedding was generated';

COMMENT ON FUNCTION search_similar_messages IS 'Searches for similar messages using cosine similarity on embeddings';
COMMENT ON FUNCTION get_unindexed_messages IS 'Returns batch of messages without embeddings (for sync job)';
COMMENT ON FUNCTION update_message_embedding IS 'Updates single message with embedding and marks as indexed';
COMMENT ON FUNCTION batch_update_embeddings IS 'Batch updates multiple messages with embeddings (atomic)';

-- =============================================================================
-- 7. Grant Permissions (if needed for service role)
-- =============================================================================

-- Grant permissions to authenticated users (adjust as needed)
-- GRANT SELECT, INSERT ON chat_messages TO authenticated;
-- GRANT EXECUTE ON FUNCTION search_similar_messages TO authenticated;

-- =============================================================================
-- Migration Complete
-- =============================================================================

-- Note: The ivfflat index works best with at least 1000 rows
-- If you're starting fresh, you can drop and recreate the index after loading data:
-- 
-- DROP INDEX IF EXISTS idx_chat_messages_embedding;
-- CREATE INDEX idx_chat_messages_embedding 
--     ON chat_messages 
--     USING ivfflat (embedding vector_cosine_ops)
--     WITH (lists = 100);
--
-- For optimal performance with large datasets (100k+ messages), consider:
-- WITH (lists = 1000) for 100k messages
-- WITH (lists = 10000) for 1M+ messages
-- Rule of thumb: lists = sqrt(rows) / 10

SELECT 'RAG migration completed successfully!' as status;
