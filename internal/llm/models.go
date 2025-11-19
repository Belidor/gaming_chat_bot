package llm

// MaxResponseLength is the maximum length for LLM response in characters
// Telegram has a limit of 4096 characters per message, so we set it to 3500
// to leave room for metadata (model name, execution time, emoji, etc.)
const MaxResponseLength = 3500

// SystemPromptTemplate is the template for system prompt with length limitation
const SystemPromptTemplate = `Ответь на следующий вопрос. ВАЖНО: твой ответ должен быть не более 3500 символов. Это строгое ограничение для совместимости с Telegram.

Вопрос: %s`

// FallbackMessage is appended when response is truncated
const FallbackMessage = "\n\n...[ответ обрезан из-за превышения лимита]"
