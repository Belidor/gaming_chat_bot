-- ============================================================================
-- КРИТИЧЕСКОЕ ИСПРАВЛЕНИЕ: batch_update_embeddings
-- ============================================================================
-- Проблема: Функция возвращает INT вместо TABLE(rows_updated INT)
-- Решение: Пересоздать функцию с правильной сигнатурой
-- ============================================================================

-- 1. Удалить старую функцию
DROP FUNCTION IF EXISTS batch_update_embeddings(BIGINT[], VECTOR(768)[]);

-- 2. Создать новую функцию с правильной сигнатурой
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
    
    -- Return as table
    RETURN QUERY SELECT updated_count;
END;
$$ LANGUAGE plpgsql;

-- 3. Проверка что функция создана правильно
SELECT 
    routine_name,
    data_type,
    type_udt_name
FROM information_schema.routines 
WHERE routine_name = 'batch_update_embeddings';

-- Должно показать: data_type = "USER-DEFINED"

SELECT 'Функция batch_update_embeddings успешно исправлена!' as status;
