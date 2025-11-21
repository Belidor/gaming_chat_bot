-- Telegram LLM Bot Database Schema
-- Database: Supabase PostgreSQL

-- Table: request_logs
-- Stores all requests made to the bot with full details
CREATE TABLE IF NOT EXISTS request_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,                    -- Telegram User ID
    username TEXT,                              -- Telegram Username (without @)
    first_name TEXT,                            -- Telegram First Name
    chat_id BIGINT NOT NULL,                    -- Telegram Chat ID
    request_text TEXT NOT NULL,                 -- User's question/request
    response_text TEXT NOT NULL,                -- LLM response
    model_used TEXT NOT NULL,                   -- 'gemini-2.0-flash' or 'gemini-2.5-pro'
    response_length INTEGER NOT NULL,           -- Length of response in characters
    execution_time_ms INTEGER NOT NULL,         -- Execution time in milliseconds
    error_message TEXT,                         -- Error message if request failed
    created_at TIMESTAMPTZ DEFAULT NOW()        -- UTC timestamp
);

-- Indexes for request_logs
CREATE INDEX IF NOT EXISTS idx_request_logs_user_id ON request_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_user_date ON request_logs(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_model_used ON request_logs(model_used);

-- Table: daily_limits
-- Tracks daily usage limits per user
CREATE TABLE IF NOT EXISTS daily_limits (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,                    -- Telegram User ID
    date DATE NOT NULL,                         -- Date in Moscow timezone (Europe/Moscow)
    pro_requests_count INTEGER DEFAULT 0,       -- Number of Pro model requests used
    flash_requests_count INTEGER DEFAULT 0,     -- Number of Flash model requests used
    updated_at TIMESTAMPTZ DEFAULT NOW(),       -- Last update timestamp
    
    CONSTRAINT unique_user_date UNIQUE(user_id, date)
);

-- Indexes for daily_limits
CREATE INDEX IF NOT EXISTS idx_daily_limits_user_id ON daily_limits(user_id);
CREATE INDEX IF NOT EXISTS idx_daily_limits_date ON daily_limits(date);
CREATE INDEX IF NOT EXISTS idx_daily_limits_user_date ON daily_limits(user_id, date);

-- Function to automatically update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to update updated_at on daily_limits
CREATE TRIGGER update_daily_limits_updated_at
    BEFORE UPDATE ON daily_limits
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Comments for documentation
COMMENT ON TABLE request_logs IS 'Stores all requests made to the Telegram bot with LLM responses';
COMMENT ON TABLE daily_limits IS 'Tracks daily request limits per user (resets at midnight Moscow time)';

COMMENT ON COLUMN request_logs.user_id IS 'Telegram user ID who made the request';
COMMENT ON COLUMN request_logs.model_used IS 'LLM model used: gemini-2.0-flash or gemini-2.5-pro';
COMMENT ON COLUMN request_logs.execution_time_ms IS 'Total execution time including API calls';
COMMENT ON COLUMN request_logs.error_message IS 'Error details if request failed';

COMMENT ON COLUMN daily_limits.date IS 'Date in Moscow timezone for daily limit tracking';
COMMENT ON COLUMN daily_limits.pro_requests_count IS 'Count of requests to Pro model (max 5 per day)';
COMMENT ON COLUMN daily_limits.flash_requests_count IS 'Count of requests to Flash model (max 25 per day)';

-- Function to atomically increment usage and return current counts
CREATE OR REPLACE FUNCTION increment_daily_limit(
    p_user_id BIGINT,
    p_date DATE,
    p_model_type TEXT
)
RETURNS TABLE(pro_count INTEGER, flash_count INTEGER) AS $$
BEGIN
    IF p_model_type = 'pro' THEN
        INSERT INTO daily_limits (user_id, date, pro_requests_count, flash_requests_count)
        VALUES (p_user_id, p_date, 1, 0)
        ON CONFLICT (user_id, date)
        DO UPDATE SET 
            pro_requests_count = daily_limits.pro_requests_count + 1,
            updated_at = NOW();
    ELSE
        INSERT INTO daily_limits (user_id, date, pro_requests_count, flash_requests_count)
        VALUES (p_user_id, p_date, 0, 1)
        ON CONFLICT (user_id, date)
        DO UPDATE SET 
            flash_requests_count = daily_limits.flash_requests_count + 1,
            updated_at = NOW();
    END IF;
    
    RETURN QUERY
    SELECT pro_requests_count, flash_requests_count
    FROM daily_limits
    WHERE user_id = p_user_id AND date = p_date;
END;
$$ LANGUAGE plpgsql;

-- Function to get current daily limit counts
CREATE OR REPLACE FUNCTION get_daily_limit(
    p_user_id BIGINT,
    p_date DATE
)
RETURNS TABLE(pro_count INTEGER, flash_count INTEGER) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        COALESCE(pro_requests_count, 0) as pro_count,
        COALESCE(flash_requests_count, 0) as flash_count
    FROM daily_limits
    WHERE user_id = p_user_id AND date = p_date;
    
    -- If no record found, return zeros
    IF NOT FOUND THEN
        RETURN QUERY SELECT 0, 0;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Optional: Create a view for daily statistics
CREATE OR REPLACE VIEW daily_statistics AS
SELECT 
    DATE(created_at AT TIME ZONE 'Europe/Moscow') as date,
    COUNT(DISTINCT user_id) as unique_users,
    COUNT(*) as total_requests,
    SUM(CASE WHEN model_used LIKE '%pro%' THEN 1 ELSE 0 END) as pro_requests,
    SUM(CASE WHEN model_used LIKE '%flash%' THEN 1 ELSE 0 END) as flash_requests,
    AVG(execution_time_ms) as avg_execution_time_ms,
    AVG(response_length) as avg_response_length
FROM request_logs
GROUP BY DATE(created_at AT TIME ZONE 'Europe/Moscow')
ORDER BY date DESC;

COMMENT ON VIEW daily_statistics IS 'Daily aggregated statistics for bot usage';

-- =============================================================================
-- RAG SYSTEM TABLES AND FUNCTIONS
-- =============================================================================

-- Enable pgvector extension for vector embeddings
CREATE EXTENSION IF NOT EXISTS vector;

COMMENT ON EXTENSION vector IS 'pgvector extension for storing and querying vector embeddings';

-- Table: chat_messages
-- Stores ALL messages from group chat for RAG search
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

-- Indexes for chat_messages
CREATE INDEX IF NOT EXISTS idx_chat_messages_user_id ON chat_messages(user_id);
CREATE INDEX IF NOT EXISTS idx_chat_messages_created_at ON chat_messages(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_chat_messages_chat_id ON chat_messages(chat_id);
CREATE INDEX IF NOT EXISTS idx_chat_messages_unindexed ON chat_messages(indexed, created_at DESC) WHERE indexed = FALSE;

-- Additional indexes for daily summary feature
CREATE INDEX IF NOT EXISTS idx_chat_messages_chat_date ON chat_messages(chat_id, created_at);
CREATE INDEX IF NOT EXISTS idx_chat_messages_chat_user_date ON chat_messages(chat_id, user_id, created_at);

-- Vector similarity index using IVFFlat algorithm (cosine distance)
-- Note: lists=100 for small datasets, adjust to sqrt(rows)/10 for large datasets
CREATE INDEX IF NOT EXISTS idx_chat_messages_embedding 
    ON chat_messages 
    USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

-- Comments for chat_messages
COMMENT ON TABLE chat_messages IS 'Stores ALL messages from group chat for RAG search';
COMMENT ON COLUMN chat_messages.message_id IS 'Original Telegram message ID';
COMMENT ON COLUMN chat_messages.message_text IS 'Full text of the message';
COMMENT ON COLUMN chat_messages.embedding IS 'Vector embedding from Gemini text-embedding-004 (768 dimensions)';
COMMENT ON COLUMN chat_messages.indexed IS 'TRUE if embedding has been generated, FALSE if pending';
COMMENT ON COLUMN chat_messages.created_at IS 'Original timestamp from Telegram message';
COMMENT ON COLUMN chat_messages.indexed_at IS 'Timestamp when embedding was generated';

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

COMMENT ON FUNCTION search_similar_messages IS 'Searches for similar messages using cosine similarity on embeddings';

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

COMMENT ON FUNCTION get_unindexed_messages IS 'Returns batch of messages without embeddings (for sync job)';

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

COMMENT ON FUNCTION update_message_embedding IS 'Updates single message with embedding and marks as indexed';

-- Function: Batch update embeddings (for efficiency)
CREATE OR REPLACE FUNCTION batch_update_embeddings(
    p_message_ids BIGINT[],
    p_embeddings VECTOR(768)[]
)
RETURNS INT AS $$
DECLARE
    rows_updated INT := 0;
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
            rows_updated := rows_updated + 1;
        END IF;
    END LOOP;
    
    RETURN rows_updated;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION batch_update_embeddings IS 'Batch updates multiple messages with embeddings (atomic)';

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
-- PERFORMANCE OPTIMIZATION NOTES
-- =============================================================================
-- For optimal ivfflat index performance with large datasets:
--   - 10k messages: WITH (lists = 100)
--   - 100k messages: WITH (lists = 1000)
--   - 1M+ messages: WITH (lists = 10000)
-- Rule of thumb: lists = sqrt(rows) / 10
--
-- To recreate index after loading data:
-- DROP INDEX IF EXISTS idx_chat_messages_embedding;
-- CREATE INDEX idx_chat_messages_embedding 
--     ON chat_messages 
--     USING ivfflat (embedding vector_cosine_ops)
--     WITH (lists = 1000);  -- Adjust based on your data size

-- =============================================================================
-- DAILY SUMMARIES
-- =============================================================================

-- Table: daily_summaries
-- Stores generated daily summaries to prevent duplicates
CREATE TABLE IF NOT EXISTS daily_summaries (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,                    -- Telegram Chat ID
    date DATE NOT NULL,                         -- Date for which summary was generated (Moscow timezone)
    summary_text TEXT NOT NULL,                 -- Generated summary with topics
    most_active_user_id BIGINT,                 -- User ID of chattiest user
    most_active_username TEXT,                  -- Username of chattiest user
    message_count INT NOT NULL,                 -- Total messages analyzed
    created_at TIMESTAMPTZ DEFAULT NOW(),       -- When summary was generated
    
    CONSTRAINT unique_chat_date UNIQUE(chat_id, date)
);

-- Indexes for daily_summaries
CREATE INDEX IF NOT EXISTS idx_daily_summaries_chat_id ON daily_summaries(chat_id);
CREATE INDEX IF NOT EXISTS idx_daily_summaries_date ON daily_summaries(date DESC);
CREATE INDEX IF NOT EXISTS idx_daily_summaries_created_at ON daily_summaries(created_at DESC);

-- Comments for daily_summaries
COMMENT ON TABLE daily_summaries IS 'Stores generated daily chat summaries';
COMMENT ON COLUMN daily_summaries.date IS 'Date for summary in Moscow timezone (summary posted next day at 7 AM)';
COMMENT ON COLUMN daily_summaries.summary_text IS 'Generated summary with topics list';
COMMENT ON COLUMN daily_summaries.most_active_user_id IS 'User who sent most messages that day';
COMMENT ON COLUMN daily_summaries.message_count IS 'Total number of messages analyzed';
