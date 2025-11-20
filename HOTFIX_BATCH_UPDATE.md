# Hotfix: Batch Update Embeddings Error

## Проблема

При выполнении `/sync` возникала ошибка:
```
json: cannot unmarshal object into Go value of type int
```

## Причина

PostgreSQL функция `batch_update_embeddings` возвращала `TABLE(rows_updated INT)`, а Supabase оборачивает это в JSON массив: `[{"rows_updated": 2}]`

Go код ожидал просто число `2`, а получал объект.

## Решение

### 1. Обновите SQL функцию в Supabase

Выполните в **Supabase SQL Editor**:

```sql
-- Drop old function
DROP FUNCTION IF EXISTS batch_update_embeddings(BIGINT[], VECTOR(768)[]);

-- Create updated function
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
```

### 2. Пересоберите и перезапустите бота

```bash
# Локально
go build -o telegram-llm-bot cmd/bot/main.go
./telegram-llm-bot

# Или Docker
docker-compose down
docker-compose up -d --build
```

### 3. Проверьте работу

Отправьте в чат:
```
/sync
```

Должно вывести:
```
✅ Синхронизация завершена!

Проиндексировано сообщений: X
Время выполнения: Y сек
```

## Статус

- ✅ SQL функция исправлена
- ✅ Go код обновлен для парсинга массива
- ✅ Бот пересобран
- ⏳ Требуется обновление функции в Supabase

## Файлы изменены

- `deployments/supabase/rag_migration.sql` - обновлена функция
- `internal/storage/chat_messages.go` - обновлен парсинг результата

---

**Дата:** 2025-11-20  
**Версия:** 1.0.1  
