# Быстрый старт: Генерация Embeddings

## Проблема была решена ✅

- **Было**: Embeddings генерировались, но 0 сохранялось в БД
- **Причина**: PostgreSQL ожидает вектор как строку `"[1.0,2.0,3.0]"`, а не массив float32
- **Решение**: Добавлена конвертация в строковый формат + исправлен парсинг JSON ответа

---

## Запустить индексацию ВСЕХ сообщений

### Вариант 1: Через Makefile (рекомендуется)

```bash
make embeddings
```

### Вариант 2: Напрямую

```bash
go run scripts/generate_embeddings.go
```

### Вариант 3: Через Docker (после пересборки)

1. Пересобрать Docker образ:
```bash
docker-compose down
docker-compose build
docker-compose up -d
```

2. Отправить `/sync` в Telegram-чат боту

---

## Примерное время выполнения

У тебя **51,234 сообщения**. При batch size 100:

- **Оптимистичная оценка** (без rate limits): ~1-2 часа
- **Реалистичная оценка** (с rate limits): ~3-5 часов
- **Безопасная оценка**: ~6-8 часов

Gemini API может применять rate limiting, поэтому лучше запустить на ночь.

---

## Мониторинг прогресса

### Вариант 1: Логи скрипта

```bash
# Запустить и следить за логами
go run scripts/generate_embeddings.go | tee embeddings.log
```

### Вариант 2: Проверка в Supabase

```sql
SELECT * FROM rag_statistics;
```

Показывает:
- `total_messages`: 51234
- `indexed_messages`: текущее количество
- `indexed_percentage`: процент готовности

### Вариант 3: Периодическая проверка

```bash
# В отдельном терминале
watch -n 60 'psql $SUPABASE_URL -c "SELECT * FROM rag_statistics;"'
```

---

## Запуск частями (если боишься rate limits)

### По 1000 сообщений за раз

```bash
make embeddings-limit LIMIT=1000
# Подожди 5 минут
make embeddings-limit LIMIT=1000
# И так далее
```

### С меньшим batch size

```bash
make embeddings-batch BATCH=50
```

---

## После завершения

1. **Проверь статистику**:
```sql
SELECT * FROM rag_statistics;
```

Должно показать `indexed_percentage = 100`.

2. **Протестируй поиск**:
Отправь боту вопрос в Telegram, он должен использовать RAG.

3. **Пересобери Docker** (если используешь):
```bash
docker-compose down
docker-compose build
docker-compose up -d
```

---

## Что делать если что-то пошло не так

### Ошибка "invalid input syntax for type vector"

**Исправлено!** Но если видишь, значит Docker использует старый код. Пересобери образ.

### Ошибка "Failed to generate embeddings"

Проблема с Gemini API:
- Проверь API ключ в `.env`
- Проверь квоты Google Cloud
- Используй меньший batch size: `-batch=20`

### Слишком медленно / Rate limits

```bash
# Медленнее, но надёжнее
go run scripts/generate_embeddings.go -batch=20
```

### Скрипт зависает

Ctrl+C и перезапусти - он продолжит с того места, где остановился (обрабатывает только неиндексированные).

---

## Запуск в фоне (на ночь)

```bash
nohup go run scripts/generate_embeddings.go > embeddings.log 2>&1 &

# Проверить что работает
tail -f embeddings.log

# Узнать PID процесса
ps aux | grep generate_embeddings

# Остановить если нужно
kill <PID>
```

---

**Создано**: 2025-11-20  
**Все исправления**: закоммичены и запушены в ветку `rag`

