# RAG System Setup Guide

## Обзор

RAG (Retrieval-Augmented Generation) система позволяет боту искать релевантную информацию из истории чата и использовать её для более точных ответов.

## Архитектура

```
┌─────────────────────────────────────────────────────────────┐
│                    Telegram Group Chat                       │
│              (ВСЕ сообщения пользователей)                   │
└────────────┬──────────────────────────────────────┬──────────┘
             │                                      │
             │ ALL messages                         │ @mention
             │ (сохраняем в chat_messages)          │
             ▼                                      ▼
┌────────────────────────┐              ┌──────────────────────┐
│  chat_messages table   │              │   Bot Handler        │
│  ├─ message_text       │              │   (обработка @)      │
│  ├─ embedding          │              └──────────┬───────────┘
│  └─ indexed: bool      │                         │
└────────────┬───────────┘                         │ QUERY
             │                                      │
             │ unindexed messages                   │ RAG Search
             ▼                                      ▼
┌────────────────────────┐              ┌──────────────────────┐
│  Nightly Sync Job      │              │   RAG Searcher       │
│  (03:00 MSK)           │              │  (vector similarity) │
│  ├─ Get unindexed      │              └──────────┬───────────┘
│  ├─ Generate embedding │                         │
│  └─ Mark indexed       │                         │ Top-5 results
└────────────┬───────────┘                         │
             │                                      │
             │ Gemini Embeddings API                │
             ▼                                      ▼
┌────────────────────────────────────────────────────┐
│           Supabase PostgreSQL + pgvector           │
│  ┌──────────────────────────────────────────────┐  │
│  │  chat_messages                               │  │
│  │  - embedding VECTOR(768)                     │  │
│  │  - ivfflat index (cosine similarity)         │  │
│  └──────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────┘
```

## Установка

### 1. Выполните SQL миграцию

В Supabase SQL Editor выполните:

```bash
cat deployments/supabase/rag_migration.sql
```

Это создаст:
- ✅ Расширение `pgvector`
- ✅ Таблицу `chat_messages` с полем `embedding VECTOR(768)`
- ✅ Индексы для быстрого поиска
- ✅ Функции для RAG операций

### 2. Настройте переменные окружения

Добавьте в ваш `.env` файл:

```env
# RAG Configuration
RAG_ENABLED=true
RAG_TOP_K=5
RAG_SIMILARITY_THRESHOLD=0.8
RAG_MAX_CONTEXT_LENGTH=2000
RAG_EMBEDDINGS_MODEL=text-embedding-004
RAG_EMBEDDINGS_BATCH_SIZE=100
```

### 3. Установите зависимости

```bash
go mod download
```

### 4. Соберите и запустите

```bash
# Сборка
make build

# Запуск
./telegram-llm-bot

# Или через Docker
docker-compose up -d
```

## Использование

### Автоматический RAG

Когда RAG включен, бот **автоматически**:

1. **Сохраняет все сообщения** из чата в таблицу `chat_messages`
2. **Ночью в 03:00 MSK** индексирует новые сообщения (создает эмбеддинги)
3. **При каждом вопросе** ищет топ-5 релевантных сообщений из истории
4. **Добавляет контекст** в промпт для LLM

### Ручная синхронизация

Запустите команду:

```
/sync
```

Это запустит индексацию всех неиндексированных сообщений прямо сейчас.

### Пример работы

**История чата:**
```
[2 дня назад] Вася: "Я использую VS Code для Go разработки"
[1 день назад] Петя: "А я предпочитаю GoLand от JetBrains"
[1 день назад] Маша: "Vim - лучший редактор!"
```

**Пользователь спрашивает:**
```
@bot какой редактор лучше для Go?
```

**RAG находит все 3 сообщения → Бот отвечает с учетом мнений из чата:**
```
Основываясь на обсуждениях в чате:
- VS Code популярен и бесплатен
- GoLand имеет отличную интеграцию с Go
- Vim для продвинутых пользователей

Рекомендую VS Code для начала, GoLand если нужны профессиональные инструменты.
```

## Конфигурация

### RAG_ENABLED
- **Тип:** boolean
- **По умолчанию:** `true`
- **Описание:** Включить/выключить RAG систему

### RAG_TOP_K
- **Тип:** int
- **По умолчанию:** `5`
- **Описание:** Количество релевантных сообщений для контекста

### RAG_SIMILARITY_THRESHOLD
- **Тип:** float64
- **По умолчанию:** `0.8`
- **Описание:** Минимальный порог similarity (0.0-1.0)
- **Рекомендация:** 
  - `0.9` - очень строгий (только очень похожие)
  - `0.8` - сбалансированный (рекомендуется)
  - `0.7` - более мягкий (больше результатов)

### RAG_MAX_CONTEXT_LENGTH
- **Тип:** int
- **По умолчанию:** `2000`
- **Описание:** Максимальная длина контекста в символах

### RAG_EMBEDDINGS_MODEL
- **Тип:** string
- **По умолчанию:** `text-embedding-004`
- **Описание:** Модель Gemini для эмбеддингов (768 dimensions)

### RAG_EMBEDDINGS_BATCH_SIZE
- **Тип:** int
- **По умолчанию:** `100`
- **Описание:** Размер батча для генерации эмбеддингов

## Мониторинг

### Статистика RAG

Выполните в Supabase SQL Editor:

```sql
-- Общая статистика
SELECT * FROM rag_statistics;

-- Результат:
-- total_messages: 5000
-- indexed_messages: 4500
-- unindexed_messages: 500
-- indexed_percentage: 90.00%
```

### Дневная статистика

```sql
SELECT * FROM daily_message_stats 
ORDER BY date DESC 
LIMIT 7;
```

### Логи бота

```bash
# Docker
docker-compose logs -f bot | grep "rag"

# Локально
# Логи выводятся в stdout
```

## Производительность

### Типичные значения

| Операция | Время |
|----------|-------|
| Сохранение сообщения | 50-100ms |
| RAG поиск | 200-500ms |
| Генерация embedding | 100-300ms |
| Batch embedding (100 msgs) | 3-10 секунд |

### Оптимизация

**Для большого объема сообщений (100k+):**

1. Настройте ivfflat индекс:
```sql
DROP INDEX IF EXISTS idx_chat_messages_embedding;
CREATE INDEX idx_chat_messages_embedding 
    ON chat_messages 
    USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 1000);  -- sqrt(100000) / 10
```

2. Увеличьте batch size:
```env
RAG_EMBEDDINGS_BATCH_SIZE=200
```

3. Используйте партиционирование по датам (для 1M+ сообщений)

## Troubleshooting

### Бот не находит релевантные сообщения

**Причины:**
1. Сообщения еще не проиндексированы
2. Слишком высокий `RAG_SIMILARITY_THRESHOLD`

**Решение:**
```bash
# Запустите ручную синхронизацию
/sync

# Или снизьте порог
RAG_SIMILARITY_THRESHOLD=0.7
```

### Ошибка "extension pgvector not found"

**Решение:**
```sql
-- Включите расширение в Supabase
CREATE EXTENSION IF NOT EXISTS vector;
```

### Медленный поиск

**Проверьте индекс:**
```sql
SELECT * FROM pg_indexes 
WHERE tablename = 'chat_messages' 
  AND indexname = 'idx_chat_messages_embedding';
```

**Пересоздайте индекс:**
```sql
DROP INDEX idx_chat_messages_embedding;
CREATE INDEX idx_chat_messages_embedding 
    ON chat_messages 
    USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
```

### Scheduler не запускается

**Проверьте timezone:**
```env
TIMEZONE=Europe/Moscow
```

**Проверьте логи:**
```bash
docker-compose logs -f bot | grep "scheduler"
```

## Безопасность

### Данные

- ✅ Все сообщения хранятся в Supabase
- ✅ Эмбеддинги генерируются через Gemini API
- ✅ Никакие персональные данные не логируются

### Costs

**Gemini Embeddings API:**
- Цена: ~$0.00025 за 1000 символов
- Для 10k сообщений по 100 символов: ~$0.25
- Для 100k сообщений: ~$2.50

**Supabase Storage:**
- pgvector индекс: ~1KB на сообщение
- 100k сообщений: ~100MB

## Дальнейшее развитие

### Возможные улучшения

1. **Function Calling** - LLM сам решает когда использовать RAG
2. **Фильтрация по времени** - искать только за последний месяц
3. **Поиск в интернете** - комбинировать RAG + web search
4. **Семантические кластеры** - группировать похожие темы
5. **Multi-query RAG** - генерировать несколько вариантов запроса

### Roadmap

- [ ] Команда `/rag_stats` для статистики RAG
- [ ] Веб-интерфейс для просмотра истории
- [ ] Экспорт истории чата
- [ ] Поддержка изображений (multimodal embeddings)

## Полезные ссылки

- [Supabase Vector Documentation](https://supabase.com/docs/guides/ai/vector-columns)
- [Gemini Embeddings API](https://ai.google.dev/docs/embeddings_guide)
- [pgvector Performance Tuning](https://github.com/pgvector/pgvector#performance)

---

**Версия:** 1.0  
**Дата:** 2025-11-20  
**Автор:** Cursor AI Agent
