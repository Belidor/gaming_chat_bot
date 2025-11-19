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
