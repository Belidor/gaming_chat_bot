package llm

// MaxResponseLength is the maximum length for LLM response in characters
// Telegram has a limit of 4096 characters per message, so we set it to 3500
// to leave room for metadata (model name, execution time, emoji, etc.)
const MaxResponseLength = 3500

// SystemPromptTemplate is the template for system prompt WITHOUT RAG context
const SystemPromptTemplate = `Ответь на следующий вопрос. ВАЖНО: твой ответ должен быть не более 3500 символов. Это строгое ограничение для совместимости с Telegram.

Вопрос: %s`

// SystemPromptWithRAGTemplate is the template for system prompt WITH RAG context
const SystemPromptWithRAGTemplate = `Ты полезный AI ассистент. У тебя есть доступ к истории чата.

%s

ВОПРОС ПОЛЬЗОВАТЕЛЯ:
%s

Ответь на вопрос, используя информацию из истории чата, если она релевантна. Если информация из истории неполная или устарела, дополни её своими знаниями.

ВАЖНО: твой ответ должен быть не более 3500 символов. Это строгое ограничение для совместимости с Telegram.`

// FallbackMessage is appended when response is truncated
const FallbackMessage = "\n\n...[ответ обрезан из-за превышения лимита]"
