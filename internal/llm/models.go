package llm

// SystemPromptTemplate is the template for system prompt with length limitation
const SystemPromptTemplate = `Ответь на следующий вопрос. ВАЖНО: твой ответ должен быть не более %d символов.

Вопрос: %s`

// FallbackMessage is appended when response is truncated
const FallbackMessage = "\n\n...[ответ обрезан из-за превышения лимита]"
