# Ручная генерация Embeddings

## Быстрый старт

### 1. Сгенерировать embeddings для всех неиндексированных сообщений

```bash
make embeddings
```

### 2. Проверить что будет обработано (без сохранения)

```bash
make embeddings-dry
```

### 3. Обработать только первые 1000 сообщений

```bash
make embeddings-limit LIMIT=1000
```

### 4. Использовать кастомный batch size

```bash
make embeddings-batch BATCH=50
```

---

## Прямой запуск скрипта

### Все неиндексированные сообщения

```bash
go run scripts/generate_embeddings.go
```

### Dry run (проверка без сохранения)

```bash
go run scripts/generate_embeddings.go -dry-run
```

### С лимитом

```bash
go run scripts/generate_embeddings.go -limit=1000
```

### Кастомный batch size

```bash
go run scripts/generate_embeddings.go -batch=50
```

### Комбинация параметров

```bash
go run scripts/generate_embeddings.go -batch=50 -limit=500
```

---

## Что делает скрипт

1. **Подключается к Supabase** и загружает неиндексированные сообщения
2. **Генерирует embeddings** через Gemini API (по батчам)
3. **Сохраняет в БД** используя функцию `batch_update_embeddings`
4. **Показывает прогресс** и статистику

---

## Параметры

| Параметр | По умолчанию | Описание |
|----------|--------------|----------|
| `-batch` | 100 | Размер батча для обработки |
| `-limit` | 0 | Максимум сообщений (0 = без лимита) |
| `-dry-run` | false | Режим проверки (не сохраняет в БД) |

---

## Примеры использования

### Быстрая проверка первых 10 сообщений

```bash
go run scripts/generate_embeddings.go -dry-run -limit=10
```

### Медленная обработка (маленький batch) для обхода rate limits

```bash
go run scripts/generate_embeddings.go -batch=20
```

### Обработать только часть (например, для тестирования)

```bash
go run scripts/generate_embeddings.go -limit=100
```

---

## Мониторинг

Скрипт выводит логи в консоль:

```
13:45:01 INF Starting embeddings generation script
13:45:01 INF Configuration loaded dry_run=false batch_size=100 limit=0
13:45:01 INF Storage client initialized
13:45:01 INF Embeddings client initialized
13:45:01 INF Starting embeddings generation...
13:45:01 INF Processing batch batch_size=100 total_processed=0
13:45:01 INF Generating embeddings... count=100
13:45:03 INF Embeddings generated successfully count=100 dimension=768
13:45:03 INF Updating database... count=100
13:45:04 INF Batch update completed updated=100 expected=100
13:45:04 INF Processing batch batch_size=100 total_processed=100
...
13:45:30 INF No more unindexed messages
13:45:30 INF Embeddings generation completed total_processed=1000 total_updated=1000
13:45:30 INF Final statistics total_messages=1000 indexed_messages=1000 indexed_percentage=100.00
```

---

## Troubleshooting

### Ошибка "Failed to update embeddings"

Проблема с БД или функцией `batch_update_embeddings`. Проверь:

```sql
-- В Supabase SQL Editor
SELECT * FROM chat_messages WHERE indexed = FALSE LIMIT 10;
```

### Ошибка "Failed to generate embeddings"

Проблема с Gemini API. Проверь:
- API ключ в `.env` (`GEMINI_API_KEY`)
- Rate limits Gemini API
- Интернет соединение

### Слишком медленно

Используй меньший batch size:

```bash
go run scripts/generate_embeddings.go -batch=50
```

Или обрабатывай частями:

```bash
go run scripts/generate_embeddings.go -limit=500
# Подожди несколько минут
go run scripts/generate_embeddings.go -limit=500
# И так далее
```

---

## Проверка результатов

После генерации проверь статистику в Supabase:

```sql
SELECT * FROM rag_statistics;
```

Должно показать:
- `total_messages`: общее количество сообщений
- `indexed_messages`: количество проиндексированных
- `indexed_percentage`: процент (должен быть 100%)

---

**Создано:** 2025-11-20  
**Автор:** Cursor AI Agent

