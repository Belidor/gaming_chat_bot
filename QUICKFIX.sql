-- QUICK FIX для batch_update_embeddings
-- Выполните этот файл в Supabase SQL Editor

-- 1. Удалите старую функцию
DROP FUNCTION IF EXISTS batch_update_embeddings(BIGINT[], VECTOR(768)[]);

-- 2. Создайте новую функцию
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

-- 3. Проверка
SELECT 'Function updated successfully!' as status;
