# Scripts - Telegram LLM Bot

## import_telegram_export.go

Утилита для импорта истории чата из Telegram Desktop JSON экспорта.

### Как получить историю чата

#### Вариант 1: Telegram Desktop Export (Рекомендуется)

1. Откройте **Telegram Desktop**
2. Зайдите в нужный чат
3. Нажмите **⋮** (три точки) → **Export chat history**
4. Выберите формат: **JSON**
5. Настройки:
   - ✅ Include media: NO (только текст)
   - ✅ Size limit: Unlimited
   - ✅ Date range: All time
6. Нажмите **Export**
7. Дождитесь завершения (файл `result.json` появится в папке)

#### Вариант 2: Python Script с Telethon (Продвинутый)

```python
# Требует установки: pip install telethon
from telethon.sync import TelegramClient

api_id = YOUR_API_ID
api_hash = 'YOUR_API_HASH'
chat_id = -1001234567890

client = TelegramClient('session', api_id, api_hash)
client.start()

messages = []
async for message in client.iter_messages(chat_id, limit=None):
    if message.text:
        messages.append({
            'id': message.id,
            'text': message.text,
            'date': message.date.isoformat(),
            'from_id': message.from_id.user_id if message.from_id else 0
        })

# Save to JSON
import json
with open('history.json', 'w', encoding='utf-8') as f:
    json.dump({'messages': messages, 'id': chat_id}, f, ensure_ascii=False)
```

### Использование import_telegram_export.go

#### 1. Dry Run (Проверка без сохранения)

```bash
go run scripts/import_telegram_export.go -file=result.json -dry-run
```

Покажет:
- Сколько сообщений найдено
- Примеры первых 5 сообщений
- Не сохранит в БД

#### 2. Импорт в БД

```bash
go run scripts/import_telegram_export.go -file=result.json
```

Выполнит:
1. Загрузит все текстовые сообщения в `chat_messages`
2. Сгенерирует embeddings для каждого сообщения
3. Обновит таблицу с embeddings

**Примерное время:**
- 1000 сообщений: ~5 минут
- 10000 сообщений: ~30 минут
- 100000 сообщений: ~5 часов

### Параметры

| Флаг | Описание | По умолчанию |
|------|----------|--------------|
| `-file` | Путь к JSON файлу | (обязательный) |
| `-dry-run` | Режим проверки (не сохранять в БД) | false |

### Формат Telegram Export

Ожидаемый формат JSON файла:

```json
{
  "name": "Chat Name",
  "type": "personal_chat",
  "id": -1001234567890,
  "messages": [
    {
      "id": 12345,
      "type": "message",
      "date": "2025-11-20T10:00:00",
      "date_unixtime": "1700478000",
      "from": "John Doe",
      "from_id": "user123456",
      "text": "Hello world"
    }
  ]
}
```

### После Импорта

Проверьте результат в Supabase:

```sql
SELECT * FROM rag_statistics;
```

Должно показать:
- total_messages: N
- indexed_messages: N
- indexed_percentage: 100%

---

## Альтернативные Методы

### Метод 1: Автоматическое накопление (Простой)

**Плюсы:** Не требует действий  
**Минусы:** Нет старой истории

Просто оставьте бота работать - он будет сохранять все новые сообщения автоматически.

### Метод 2: Manual CSV Import (Для больших объемов)

Если у вас очень большая история (500k+ сообщений):

1. Экспортируйте в CSV
2. Используйте Supabase Dashboard → Table Editor → Import CSV
3. Затем запустите:
   ```sql
   UPDATE chat_messages SET indexed = FALSE;
   ```
4. Запустите несколько раз `/sync` для индексации

---

## Troubleshooting

### Ошибка "Failed to parse export JSON"

Убедитесь что:
- Файл в кодировке UTF-8
- Формат JSON корректный
- Используется экспорт из Telegram Desktop (не других клиентов)

### Слишком долго генерируются embeddings

Нормально для больших объемов. Gemini Embeddings API имеет rate limit.

Можно:
- Разбить на несколько запусков
- Использовать меньший batch size
- Запускать ночью

---

**Создано:** 2025-11-20  
**Автор:** Cursor AI Agent
