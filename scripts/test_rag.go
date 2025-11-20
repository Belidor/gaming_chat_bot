package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/telegram-llm-bot/internal/config"
	"github.com/telegram-llm-bot/internal/embeddings"
	"github.com/telegram-llm-bot/internal/rag"
	"github.com/telegram-llm-bot/internal/storage"
)

func main() {
	// Parse flags
	query := flag.String("query", "", "Search query")
	topK := flag.Int("top", 5, "Number of results to return")
	threshold := flag.Float64("threshold", 0.7, "Similarity threshold (0.0-1.0)")
	flag.Parse()

	// Setup logging
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"})

	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Warn().Err(err).Msg("No .env file found, using environment variables")
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	// Initialize clients
	storageClient, err := storage.NewClient(
		cfg.SupabaseURL,
		cfg.SupabaseKey,
		cfg.SupabaseTimeout,
		log.Logger,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize storage client")
	}

	embeddingsClient := embeddings.NewClient(
		cfg.GeminiAPIKey,
		cfg.RAG.EmbeddingsModel,
		cfg.RAG.EmbeddingsBatchSize,
		time.Duration(cfg.GeminiTimeout)*time.Second,
		log.Logger,
	)

	// Override RAG config for testing
	ragConfig := cfg.RAG
	ragConfig.TopK = *topK
	ragConfig.SimilarityThreshold = *threshold
	
	ragSearcher := rag.NewSearcher(storageClient, embeddingsClient, ragConfig, log.Logger)

	ctx := context.Background()

	// Get statistics first
	fmt.Println("\n=== RAG Statistics ===")
	stats, err := storageClient.GetRAGStatistics(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get statistics")
	} else {
		fmt.Printf("Total messages: %.0f\n", stats["total_messages"])
		fmt.Printf("Indexed messages: %.0f\n", stats["indexed_messages"])
		fmt.Printf("Indexed percentage: %.2f%%\n", stats["indexed_percentage"])
		fmt.Printf("Last indexing: %v\n", stats["last_indexing"])
		fmt.Println()
	}

	// If no query provided, show examples
	if *query == "" {
		fmt.Println("=== Example Queries ===")
		fmt.Println("Based on typical gaming/crypto chat history:\n")
		
		examples := []string{
			"Какие игры обсуждали?",
			"Что говорили про крипту?",
			"Обсуждение турниров",
			"Рекомендации по настройкам",
			"Проблемы с лагами",
			"Что говорили про NFT?",
			"Обсуждение патчей",
			"Какие команды упоминали?",
		}
		
		for _, ex := range examples {
			fmt.Printf("  go run scripts/test_rag.go -query=\"%s\"\n", ex)
		}
		fmt.Println("\nUsage:")
		fmt.Println("  go run scripts/test_rag.go -query=\"ваш вопрос\" [-top=5] [-threshold=0.7]")
		return
	}

	// Perform search (using chatID from first allowed chat)
	chatID := cfg.AllowedChatIDs[0]
	
	fmt.Printf("\n=== Searching for: \"%s\" ===\n\n", *query)
	fmt.Printf("Parameters: top_k=%d, threshold=%.2f\n\n", *topK, *threshold)

	startTime := time.Now()
	result, err := ragSearcher.Search(ctx, *query, chatID)
	duration := time.Since(startTime)

	if err != nil {
		log.Fatal().Err(err).Msg("Search failed")
	}

	fmt.Printf("Found %d results in %v\n\n", len(result.Messages), duration)

	if len(result.Messages) == 0 {
		fmt.Println("❌ No results found. Try:")
		fmt.Println("  - Lowering threshold: -threshold=0.5")
		fmt.Println("  - Increasing top_k: -top=10")
		fmt.Println("  - Using different query")
		return
	}

	// Display results
	for i, msg := range result.Messages {
		fmt.Printf("─────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("Result #%d\n", i+1)
		fmt.Printf("─────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("User: %s (@%s)\n", msg.FirstName, msg.Username)
		fmt.Printf("Date: %s\n", msg.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Message ID: %d\n", msg.MessageID)
		fmt.Printf("\nText:\n%s\n\n", msg.MessageText)
	}

	// Show context for LLM
	fmt.Println("\n=== Context for LLM ===")
	fmt.Println("(This is what would be added to LLM prompt)\n")
	
	fmt.Println(result.Context)
	fmt.Printf("\n(Context length: %d characters)\n", len(result.Context))
}

