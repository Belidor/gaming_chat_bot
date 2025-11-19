# Telegram LLM Bot - Техническая спецификация

## Обзор проекта

Telegram бот на Go для группового чата с интеграцией Google Gemini AI, системой rate limiting и полным логированием запросов в Supabase PostgreSQL.

## Архитектура

### High-Level Architecture

```
┌─────────────┐
│  Telegram   │
│   Users     │
└──────┬──────┘
       │ @mention
       ▼
┌─────────────────────────────────────────┐
│         Telegram Bot API                │
│  (github.com/go-telegram-bot-api)       │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│          Bot Handler                    │
│  - Message routing                      │
│  - Command processing                   │
│  - Middleware (recovery, logging)       │
└──────┬──────────────────────────────────┘
       │
       ├──────────────┐
       ▼              ▼
┌──────────────┐  ┌──────────────┐
│ Rate Limiter │  │  LLM Client  │
│  (Supabase)  │  │   (Gemini)   │
└──────┬───────┘  └──────┬───────┘
       │                 │
       ▼                 ▼
┌─────────────────────────────────┐
│       Supabase PostgreSQL       │
│  - request_logs                 │
│  - daily_limits                 │
└─────────────────────────────────┘
```

### Компоненты системы

#### 1. Bot Package (`internal/bot`)
Основной пакет для работы с Telegram Bot API.

**Файлы:**
- `bot.go` - Инициализация и запуск бота
- `handler.go` - Обработка сообщений, команд и упоминаний
- `middleware.go` - Middleware функции (recovery, отправка сообщений)

**Ключевые функции:**
- `New()` - Создание экземпляра бота
- `Start()` - Запуск polling цикла
- `handleUpdate()` - Обработка входящих обновлений
- `handleMention()` - Обработка упоминаний бота
- `handleCommand()` - Обработка команд (/stats, /help)

#### 2. LLM Package (`internal/llm`)
Клиент для работы с Google Gemini API.

**Файлы:**
- `client.go` - Основная логика взаимодействия с API
- `models.go` - Константы промптов

**Особенности:**
- Автоматический retry при ошибках (3 попытки, exponential backoff)
- Встроенное ограничение длины ответа в промпте
- Fallback обрезка если LLM не соблюдает лимит
- Контекст с таймаутом для всех запросов

**Модели:**
- `gemini-2.5-pro` (Pro) - Модель для сложных задач (2 RPM, 50 RPD)
- `gemini-2.0-flash` (Flash) - Быстрая модель (15 RPM, 200 RPD)

#### 3. Rate Limit Package (`internal/ratelimit`)
Управление лимитами запросов пользователей.

**Файлы:**
- `limiter.go` - Логика проверки и обновления лимитов
- `models.go` - Вспомогательные типы

**Логика работы:**
1. Проверяет текущую дату в Moscow timezone
2. Получает счетчики использования из БД
3. Определяет доступную модель (Pro → Flash)
4. После успешного запроса инкрементирует счетчик

**Лимиты:**
- Pro модель: 5 запросов/день
- Flash модель: 25 запросов/день
- Сброс в полночь Moscow time

#### 4. Storage Package (`internal/storage`)
Взаимодействие с Supabase PostgreSQL.

**Файлы:**
- `supabase.go` - Клиент и базовая логика
- `requests.go` - Работа с таблицей request_logs
- `limits.go` - Работа с таблицей daily_limits

**Возможности:**
- Retry логика (2 попытки с backoff)
- Ping для проверки подключения
- Upsert для атомарного обновления лимитов

#### 5. Config Package (`internal/config`)
Загрузка и валидация конфигурации.

**Файлы:**
- `config.go` - Загрузка из .env и валидация

**Валидация:**
- Обязательные параметры (токены, API ключи)
- Положительные значения для лимитов и таймаутов
- Корректный log level

#### 6. Models Package (`internal/models`)
Общие типы данных для всего приложения.

**Основные типы:**
- `BotConfig` - Конфигурация приложения
- `RequestLog` - Лог запроса в БД
- `DailyLimit` - Дневные лимиты пользователя
- `UserStats` - Статистика пользователя
- `LLMRequest/Response` - Запрос/ответ LLM
- `RateLimitResult` - Результат проверки лимита

## База данных

### Таблица: request_logs

```sql
CREATE TABLE request_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    username TEXT,
    first_name TEXT,
    chat_id BIGINT NOT NULL,
    request_text TEXT NOT NULL,
    response_text TEXT NOT NULL,
    model_used TEXT NOT NULL,
    response_length INTEGER NOT NULL,
    execution_time_ms INTEGER NOT NULL,
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

**Индексы:**
- `idx_request_logs_user_id` - Быстрый поиск по user_id
- `idx_request_logs_created_at` - Сортировка по дате
- `idx_request_logs_user_date` - Композитный индекс
- `idx_request_logs_model_used` - Фильтрация по модели

### Таблица: daily_limits

```sql
CREATE TABLE daily_limits (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    date DATE NOT NULL,
    pro_requests_count INTEGER DEFAULT 0,
    flash_requests_count INTEGER DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT unique_user_date UNIQUE(user_id, date)
);
```

**Constraint:**
- `unique_user_date` - Один лимит на пользователя на дату

**Trigger:**
- `update_daily_limits_updated_at` - Автообновление updated_at

### View: daily_statistics

Агрегированная статистика по дням для аналитики.

## Флоу обработки запроса

### 1. Получение сообщения

```
User → Telegram → Bot API → handleUpdate() → handleMessage()
```

### 2. Проверка упоминания

```go
if message.Chat.ID != config.GroupChatID {
    return // Игнорируем сообщения из других чатов
}

if !isMentioned(message) {
    return // Игнорируем сообщения без упоминания
}
```

### 3. Проверка лимитов

```go
limitResult := limiter.CheckLimit(ctx, userID)

if !limitResult.Allowed {
    sendMessage("Лимит исчерпан")
    return
}

modelToUse := limitResult.ModelToUse // Pro или Flash
```

### 4. Запрос к LLM

```go
llmReq := &LLMRequest{
    Text: questionText,
    ModelType: modelToUse,
    // MaxLength жестко закодирована в промпте как 3500 символов
    // для совместимости с лимитом Telegram (4096 символов)
}

llmResp := llmClient.GenerateResponse(ctx, llmReq)
```

### 5. Обработка ответа

```go
if llmResp.Error != nil {
    logFailedRequest()
    sendErrorMessage()
    return // НЕ списываем с лимита!
}

// Списываем с лимита
limiter.IncrementUsage(userID, modelToUse)

// Логируем успешный запрос
storage.LogRequest(requestLog)

// Отправляем ответ
sendMessage(llmResp.Text)
```

## Обработка ошибок

### Стратегия retry

**LLM запросы:**
- 3 попытки
- Exponential backoff: 1s, 2s, 4s
- Timeout: 30 секунд
- Ошибка НЕ списывается с лимита

**Supabase запросы:**
- 2 попытки
- Backoff: 500ms, 1s
- Timeout: 10 секунд
- Ошибка НЕ списывается с лимита

### Типы ошибок

1. **User errors** - Неверный формат, неизвестная команда
   - Отправляем help message
   
2. **API errors** - Недоступность Gemini/Supabase
   - Retry с backoff
   - Информируем пользователя
   
3. **System errors** - Паники, критические ошибки
   - Recovery middleware
   - Логируем с полным stack trace
   - Отправляем generic error message

## Безопасность

### Защита credentials
- Все секреты в environment variables
- Никогда не логируются API ключи/токены
- .env в .gitignore

### Docker security
- Non-root пользователь в контейнере
- Minimal Alpine base image
- No unnecessary packages

### Rate limiting
- Защита от спама через дневные лимиты
- Групповой чат защищен проверкой Chat ID
- Каждый пользователь независим

## Performance

### Оптимизации

1. **Горутины для обработки сообщений**
   - Каждое сообщение в отдельной горутине
   - Не блокируем основной цикл polling

2. **Контексты с таймаутами**
   - Предотвращают зависание на долгих операциях
   - Gemini: 30s, Supabase: 10s

3. **Индексы БД**
   - Быстрые запросы по user_id и date
   - Композитные индексы для сложных фильтров

4. **Эффективное логирование**
   - Structured logging (JSON в production)
   - Асинхронная запись в БД

### Метрики

Типичные значения:
- Ответ Gemini Pro: 2-5 секунд
- Ответ Gemini Flash: 1-3 секунды
- Запрос в Supabase: 50-200ms
- Размер Docker образа: ~20-30MB

## Deployment

### Требования к окружению

**Минимальные ресурсы:**
- CPU: 0.5 core
- RAM: 256MB
- Disk: 100MB

**Рекомендуемые:**
- CPU: 1 core
- RAM: 512MB
- Disk: 1GB (с запасом под логи)

### Переменные окружения

См. `.env.example` для полного списка.

**Обязательные:**
- TELEGRAM_BOT_TOKEN
- TELEGRAM_BOT_USERNAME
- TELEGRAM_ALLOWED_CHAT_IDS (comma-separated list of allowed chat IDs)
- GEMINI_API_KEY
- SUPABASE_URL
- SUPABASE_KEY

### Запуск

```bash
# Docker Compose (рекомендуется)
docker-compose up -d

# Проверка
docker-compose logs -f
docker-compose ps
```

### Обновление

```bash
git pull
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

### Мониторинг

**Логи:**
```bash
docker-compose logs -f bot
```

**Статистика в БД:**
```sql
SELECT * FROM daily_statistics;
SELECT COUNT(*) FROM request_logs WHERE created_at > NOW() - INTERVAL '24 hours';
```

## Тестирование

### Ручное тестирование

1. Создайте тестовый чат
2. Добавьте бота
3. Отправьте `@botname test question`
4. Проверьте `/stats`
5. Исчерпайте лимиты и проверьте сообщения

### Unit тесты (TODO)

```bash
go test -v ./...
go test -cover ./...
```

### Integration тесты (TODO)

Требуют:
- Тестовый Telegram аккаунт
- Тестовый Supabase проект
- Gemini API ключ с квотой

## Ограничения и известные проблемы

### Текущие ограничения

1. **Один чат**: Бот работает только в одном групповом чате
2. **Нет контекста**: Каждый запрос независим, история не сохраняется
3. **Простой rate limiting**: Только по количеству, нет защиты от быстрых запросов
4. **Отсутствие кэширования**: Повторяющиеся вопросы отправляются в LLM

### Известные проблемы

1. **Timezone sync**: Важно чтобы системное время было синхронизировано
2. **Gemini API quota**: Нет предупреждений о приближении к квоте API
3. **Long polling**: При сетевых проблемах может быть задержка до 60 секунд

## Roadmap / Будущие улучшения

### Высокий приоритет
- [ ] Добавить unit и integration тесты
- [ ] Реализовать кэширование частых вопросов
- [ ] Добавить Prometheus metrics

### Средний приоритет
- [ ] Поддержка нескольких чатов
- [ ] Whitelist пользователей
- [ ] Веб-интерфейс для статистики
- [ ] History/контекст разговора

### Низкий приоритет
- [ ] Webhook вместо long polling
- [ ] Поддержка изображений
- [ ] Голосовые сообщения → text-to-speech
- [ ] Платные подписки с расширенными лимитами

## Глоссарий

- **Pro модель** - Gemini 2.0 Flash Thinking Experimental, более медленная но более точная
- **Flash модель** - Gemini 2.0 Flash Experimental, быстрая модель
- **Rate limiting** - Ограничение количества запросов пользователя
- **Graceful shutdown** - Корректное завершение с ожиданием активных операций
- **Long polling** - Способ получения обновлений от Telegram (альтернатива webhook)
- **Upsert** - Операция INSERT or UPDATE в БД

## Ссылки

- [Telegram Bot API](https://core.telegram.org/bots/api)
- [Google Gemini API](https://ai.google.dev/docs)
- [Supabase Documentation](https://supabase.com/docs)
- [Go Telegram Bot API Library](https://github.com/go-telegram-bot-api/telegram-bot-api)
- [Generative AI Go SDK](https://github.com/google/generative-ai-go)

---

**Версия документа:** 1.0  
**Дата обновления:** 2025-11-18  
**Автор:** Cursor AI Agent
