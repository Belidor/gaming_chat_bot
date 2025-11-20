# Применение исправлений для RAG поиска

## Проблема

RAG поиск не находил импортированные из Telegram Desktop сообщения из-за несоответствия форматов:

1. **Chat ID**: `-1001750074031` (Bot API) vs `1750074031` (экспорт)
2. **User ID**: Все импортированные сообщения имели `user_id = 0`

## Быстрое решение (3 шага)

### Шаг 1: Исправить chat_id в базе данных

1. Откройте [Supabase SQL Editor](https://app.supabase.com)
2. Выберите ваш проект
3. Скопируйте и выполните SQL из файла:

```bash
deployments/supabase/fix_chat_id.sql
```

**Что делает:**
- Конвертирует положительные chat_id в формат Bot API
- Пример: `1750074031` → `-1001750074031`

### Шаг 2: Исправить user_id в базе данных

1. В том же SQL Editor
2. Скопируйте и выполните SQL из файла:

```bash
deployments/supabase/fix_user_id.sql
```

**Что делает:**
- Находит сообщения с `user_id = 0`
- Сопоставляет их по username с существующими сообщениями
- Обновляет user_id там, где возможно

### Шаг 3: Проверить работу RAG

1. Упомяните бота в групповом чате:
   ```
   @your_bot что обсуждали по поводу работы?
   ```

2. Проверьте, что бот отвечает с контекстом из истории

3. Проверьте логи:
   ```bash
   docker-compose logs bot | grep "RAG search completed"
   ```

   Должно быть: `results_count > 0`

## Проверка результатов

### В Supabase SQL Editor выполните:

```sql
-- Проверка chat_id
SELECT 
    chat_id,
    COUNT(*) as message_count,
    COUNT(CASE WHEN indexed = TRUE THEN 1 END) as indexed_count
FROM chat_messages
GROUP BY chat_id
ORDER BY chat_id;
```

**Ожидаемый результат:**
- Все supergroup сообщения имеют отрицательный chat_id (например, `-1001750074031`)

```sql
-- Проверка user_id
SELECT 
    CASE WHEN user_id = 0 THEN 'Unknown' ELSE 'Known' END as user_type,
    COUNT(*) as message_count,
    COUNT(DISTINCT username) as unique_users
FROM chat_messages
GROUP BY user_type;
```

**Ожидаемый результат:**
- Большинство сообщений должны иметь `user_id != 0`
- Сообщения с `user_id = 0` - это только те пользователи, которые есть в истории, но не использовали бота после его запуска

## Что дальше?

После применения исправлений:

✅ **RAG поиск заработает** - бот будет находить релевантные сообщения из истории

✅ **Новые импорты** - скрипт `import_telegram_export.go` теперь автоматически нормализует chat_id и парсит user_id

✅ **Не нужно повторять** - исправления применяются один раз

## Если что-то пошло не так

### RAG все равно не работает?

1. Проверьте chat_id бота:
   ```bash
   docker-compose logs bot | grep "chat_id"
   ```

2. Проверьте, что chat_id в базе совпадает с chat_id из логов

3. Проверьте настройки RAG в `.env`:
   ```
   RAG_ENABLED=true
   RAG_TOP_K=5
   RAG_SIMILARITY_THRESHOLD=0.8
   ```

### Нужна помощь?

Смотрите подробную документацию: [CHAT_ID_FIX.md](CHAT_ID_FIX.md)

## Откат (не рекомендуется)

Если нужно откатить изменения:

```sql
-- ВНИМАНИЕ: Это сломает RAG для текущих сообщений!
UPDATE chat_messages
SET chat_id = ABS(chat_id + 1000000000000)
WHERE chat_id < -1000000000000;
```

Используйте только для восстановления данных в случае критической ошибки.

