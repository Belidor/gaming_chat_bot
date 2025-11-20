#!/bin/bash
# Fix Chat IDs in Database
# This script normalizes chat_id format from Telegram Desktop export format
# to Telegram Bot API format

set -e

echo "🔧 Fixing Chat IDs in Database..."
echo ""

# Load environment variables
if [ -f .env ]; then
    source .env
else
    echo "❌ Error: .env file not found"
    exit 1
fi

# Check if SUPABASE_URL and SUPABASE_KEY are set
if [ -z "$SUPABASE_URL" ] || [ -z "$SUPABASE_KEY" ]; then
    echo "❌ Error: SUPABASE_URL and SUPABASE_KEY must be set in .env"
    exit 1
fi

echo "📊 Current state of chat_messages:"
echo ""

# Show current state using Supabase REST API
curl -s "$SUPABASE_URL/rest/v1/rpc/custom_query" \
  -H "apikey: $SUPABASE_KEY" \
  -H "Authorization: Bearer $SUPABASE_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "SELECT chat_id, COUNT(*) as count FROM chat_messages GROUP BY chat_id ORDER BY chat_id"
  }' || echo "Note: Custom query endpoint might not be available, continuing..."

echo ""
echo "🔄 Applying chat_id normalization..."
echo ""
echo "Please run the following SQL in your Supabase SQL Editor:"
echo ""
cat deployments/supabase/fix_chat_id.sql
echo ""
echo "Or use psql:"
echo ""
echo "  psql \$DATABASE_URL < deployments/supabase/fix_chat_id.sql"
echo ""
echo "After running the SQL, verify the changes and test RAG search."

