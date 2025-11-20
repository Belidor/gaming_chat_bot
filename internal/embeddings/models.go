package embeddings

// EmbeddingModel constants
const (
	// ModelTextEmbedding004 is the latest Gemini embedding model (768 dimensions)
	ModelTextEmbedding004 = "text-embedding-004"

	// ModelEmbedding001 is the legacy embedding model (768 dimensions)
	ModelEmbedding001 = "embedding-001"
)

// DefaultModel is the default embedding model to use
const DefaultModel = ModelTextEmbedding004

// DefaultBatchSize is the default batch size for embedding generation
// Gemini allows up to 100 texts per batch
const DefaultBatchSize = 100

// DefaultTimeout is the default timeout for embedding generation
const DefaultTimeout = 30 // seconds
