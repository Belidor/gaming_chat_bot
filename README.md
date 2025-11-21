# Telegram LLM Bot

A production-ready Telegram bot powered by Google Gemini AI with RAG (Retrieval-Augmented Generation) and automated daily summaries for group chat management.

## Features

- **Google Gemini Integration**: Dual-model support (Gemini 2.0 Flash Thinking and Gemini 2.0 Flash)
- **RAG System**: Vector search over entire chat history using pgvector and embeddings
- **Context-Aware Responses**: Bot uses past discussions for more relevant answers
- **Daily Summaries**: Automated chat summaries posted every morning at 7 AM MSK
- **Smart Rate Limiting**: 5 Pro requests/day, 25 Flash requests/day per user
- **Automatic Indexing**: Nightly synchronization of new messages (03:00 MSK)
- **Supabase Integration**: PostgreSQL database with vector search capabilities
- **Docker Support**: Full containerization for easy deployment
- **Graceful Shutdown**: Proper cleanup on termination
- **Structured Logging**: JSON logging with zerolog

## Quick Start

### Prerequisites

- Docker and Docker Compose (recommended) or Go 1.21+
- Telegram Bot Token (from [@BotFather](https://t.me/BotFather))
- Google Gemini API Key (from [Google AI Studio](https://makersuite.google.com/app/apikey))
- Supabase account (from [Supabase](https://supabase.com))

### Installation

1. **Clone the repository**

```bash
git clone <repository-url>
cd gaming_chat_bot
```

2. **Set up Supabase database**

Create a new project on Supabase and execute the complete schema:

```sql
-- Execute in Supabase SQL Editor
-- Copy and run the entire: deployments/supabase/schema.sql
```

This single file creates all necessary tables, indexes, functions, and views:
- `request_logs` - LLM request history
- `daily_limits` - Rate limiting per user
- `chat_messages` - All messages with vector embeddings for RAG
- `daily_summaries` - Generated daily chat summaries
- pgvector extension and all required functions

3. **Configure environment variables**

```bash
cp .env.example .env
nano .env
```

Required configuration:

```env
# Telegram Bot Configuration
TELEGRAM_BOT_TOKEN=your_bot_token_from_botfather
TELEGRAM_BOT_USERNAME=your_bot_username
TELEGRAM_ALLOWED_CHAT_IDS=-1001234567890  # Comma-separated chat IDs

# Google Gemini API
GEMINI_API_KEY=your_gemini_api_key

# Supabase Configuration
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_KEY=your_supabase_anon_or_service_key

# RAG Configuration
RAG_ENABLED=true
RAG_TOP_K=5
RAG_SIMILARITY_THRESHOLD=0.8

# Summary Configuration
SUMMARY_ENABLED=true
SUMMARY_TIME=07:00
```

4. **Run the bot**

**Option A: Docker Compose (recommended)**

```bash
docker-compose up -d
docker-compose logs -f
```

**Option B: Local development**

```bash
go mod download
go run cmd/bot/main.go
```

5. **Initial setup**

After the bot starts, in your Telegram chat:

```
# Index existing messages for RAG
/sync
```

## Usage

### Bot Commands

- `/start` or `/help` - Show help message
- `/stats` - Display your usage statistics
- `/sync` - Manually trigger message indexing for RAG

### Asking Questions

Mention the bot in your group chat:

```
@your_bot_username what is quantum physics?
```

The bot responds using available AI models and incorporates relevant chat history via RAG.

### Daily Summaries

Every day at 7:00 AM MSK, the bot automatically posts a summary of the previous day's discussion:

```
📊 Summary for January 15, 2025

🗣️ Most discussed topics:
• Project deadlines and milestones
• New feature implementation details
• Bug fixes in production

💬 Total messages: 247
🏆 Most active: @username (42 messages)
```

### Example Interaction

```
User: @mybot what did we discuss about Go yesterday?
Bot: Based on the chat history:
     - Alex mentioned using VS Code for Go development
     - Maria recommended GoLand for its debugging features
     - The team discussed migrating to Go 1.21
     
     🤖 Model: gemini-2.0-flash | Time: 1800ms

User: /stats
Bot: 📊 Statistics for John
     
     🤖 Gemini Pro (Thinking):
        Used: 3/5
        Remaining: 2
     
     ⚡ Gemini Flash:
        Used: 5/25
        Remaining: 20
     
     📈 Total requests: 45
     ⏰ Limits reset in: 8 hours
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | Yes | - | Bot token from BotFather |
| `TELEGRAM_BOT_USERNAME` | Yes | - | Bot username without @ |
| `TELEGRAM_ALLOWED_CHAT_IDS` | Yes | - | Comma-separated allowed chat IDs |
| `GEMINI_API_KEY` | Yes | - | Google Gemini API key |
| `SUPABASE_URL` | Yes | - | Supabase project URL |
| `SUPABASE_KEY` | Yes | - | Supabase API key |
| `TIMEZONE` | No | `Europe/Moscow` | Timezone for schedules |
| `LOG_LEVEL` | No | `info` | Logging level |
| `ENVIRONMENT` | No | `production` | Environment name |
| `PRO_DAILY_LIMIT` | No | `5` | Daily Pro model requests |
| `FLASH_DAILY_LIMIT` | No | `25` | Daily Flash model requests |
| `RAG_ENABLED` | No | `true` | Enable RAG system |
| `RAG_TOP_K` | No | `5` | Number of relevant messages |
| `RAG_SIMILARITY_THRESHOLD` | No | `0.8` | Similarity score (0.0-1.0) |
| `SUMMARY_ENABLED` | No | `true` | Enable daily summaries |
| `SUMMARY_TIME` | No | `07:00` | Time to post summaries (HH:MM) |

### Getting Chat ID

1. Add your bot to a group
2. Send any message in the group
3. Visit: `https://api.telegram.org/bot<YOUR_BOT_TOKEN>/getUpdates`
4. Find `"chat":{"id":-1001234567890}` in the response
5. Use this ID in `TELEGRAM_ALLOWED_CHAT_IDS`

## RAG System

The bot uses Retrieval-Augmented Generation for context-aware responses.

### How It Works

1. **Collection**: All chat messages are automatically saved
2. **Indexing**: Messages converted to vector embeddings (Gemini text-embedding-004)
3. **Retrieval**: Top-K relevant messages found using cosine similarity
4. **Augmentation**: Retrieved context added to LLM prompt
5. **Generation**: Gemini generates informed response

### Architecture

```
User Message → Save to DB
                    ↓
            Nightly Sync (03:00 MSK)
                    ↓
         Generate Embeddings → Store Vectors
                    ↓
User Question → RAG Search (Top-5 Similar)
                    ↓
         Context + Question → LLM
                    ↓
              Contextual Response
```

### Performance

- **Vector Search**: ~200-500ms for 50k+ messages
- **Embedding Generation**: ~100-300ms per message
- **Batch Processing**: 100 messages per batch
- **Storage**: ~1KB per message (text + 768-dim vector)

## Daily Summaries

Automated chat digests posted every morning.

### Features

- **Topic Analysis**: Identifies main discussion themes
- **Activity Stats**: Message counts and active users
- **Most Active User**: Recognizes top contributor
- **Duplicate Prevention**: One summary per day per chat
- **Configurable Schedule**: Adjust posting time via `SUMMARY_TIME`

### How It Works

1. At 7:00 AM MSK, bot analyzes previous day's messages
2. Generates summary using Gemini with context from all messages
3. Identifies most active participant
4. Posts formatted summary to chat
5. Stores in database to prevent regeneration

## Development

### Project Structure

```
gaming_chat_bot/
├── cmd/bot/              # Application entry point
├── internal/
│   ├── bot/              # Telegram bot logic
│   ├── config/           # Configuration management
│   ├── embeddings/       # Gemini embeddings client
│   ├── llm/              # Gemini LLM client
│   ├── models/           # Data structures
│   ├── ratelimit/        # Rate limiting logic
│   ├── scheduler/        # Cron job scheduler
│   ├── storage/          # Supabase integration
│   └── summary/          # Summary generation
├── deployments/supabase/ # Complete database schema
├── scripts/              # Utility scripts
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

### Building

```bash
# Build binary
make build

# Build Docker image
make docker-build

# Run tests
make test

# Format code
go fmt ./...
```

### Database Schema

**Tables:**
- `request_logs`: All user requests and responses
- `daily_limits`: Per-user daily rate limits
- `chat_messages`: All messages with vector embeddings
- `daily_summaries`: Generated daily chat summaries

**Key Functions:**
- `get_daily_limit(user_id, date)`: Get current user limits
- `increment_daily_limit(user_id, date, model)`: Atomic limit increment
- `search_similar_messages(query_embedding, top_k, threshold)`: Vector search
- `get_unindexed_messages(batch_size)`: Get messages pending indexing
- `batch_update_embeddings(ids[], embeddings[])`: Batch embedding updates

**Views:**
- `daily_statistics`: Aggregated bot usage stats
- `rag_statistics`: RAG indexing status
- `daily_message_stats`: Daily message statistics

See `SPECIFICATION.md` for detailed technical documentation.

## Monitoring

### View Logs

```bash
# Docker Compose
docker-compose logs -f

# Docker
docker logs -f telegram-llm-bot
```

### Database Queries

```sql
-- Overall RAG statistics
SELECT * FROM rag_statistics;

-- Recent summaries
SELECT * FROM daily_summaries 
ORDER BY date DESC 
LIMIT 7;

-- Daily message statistics
SELECT * FROM daily_message_stats 
ORDER BY date DESC 
LIMIT 7;

-- Top users by requests
SELECT username, COUNT(*) as total
FROM request_logs
GROUP BY username
ORDER BY total DESC
LIMIT 10;
```

## Troubleshooting

### Bot doesn't respond

- Verify bot is added to the group
- Check `TELEGRAM_ALLOWED_CHAT_IDS` contains the correct chat ID
- Review logs: `docker-compose logs -f`

### RAG not finding relevant messages

- Run `/sync` to index messages
- Lower `RAG_SIMILARITY_THRESHOLD` to 0.6-0.7
- Check indexing: `SELECT * FROM rag_statistics;` in Supabase

### Daily summary not posting

- Verify `SUMMARY_ENABLED=true` in config
- Check scheduler logs for errors
- Ensure bot has permission to post in chat
- Verify timezone setting matches expected schedule

### Rate limit errors

- Verify Gemini API quota in Google Cloud Console
- Check Supabase connection and limits
- Review error logs for specific issues

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License

---

**Note**: This bot is designed for private group chats. For public use, implement additional security measures and content moderation.
