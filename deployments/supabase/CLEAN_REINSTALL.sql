-- ============================================================================
-- CLEAN REINSTALL - RAG System
-- ============================================================================
-- This script completely removes all RAG components and reinstalls them
-- WARNING: This will DELETE all messages from chat_messages table!
-- ============================================================================

-- Step 1: Drop all existing objects
-- ============================================================================

-- Drop views
DROP VIEW IF EXISTS rag_statistics CASCADE;
DROP VIEW IF EXISTS daily_message_stats CASCADE;

-- Drop functions
DROP FUNCTION IF EXISTS search_similar_messages(VECTOR(768), FLOAT, INT, BIGINT) CASCADE;
DROP FUNCTION IF EXISTS get_unindexed_messages(INT) CASCADE;
DROP FUNCTION IF EXISTS update_message_embedding(BIGINT, VECTOR(768)) CASCADE;
DROP FUNCTION IF EXISTS batch_update_embeddings(BIGINT[], VECTOR(768)[]) CASCADE;

-- Drop table (WARNING: deletes all data!)
DROP TABLE IF EXISTS chat_messages CASCADE;

SELECT 'Step 1 complete: All RAG objects dropped' as status;

-- ============================================================================
-- Step 2: Now execute the full rag_migration.sql file
-- ============================================================================
-- After running this script, copy and paste the ENTIRE content of:
-- deployments/supabase/rag_migration.sql
-- ============================================================================
