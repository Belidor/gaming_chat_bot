# Chat ID and User ID Fix

## Problems

RAG search was not finding messages imported from Telegram Desktop JSON exports. There were TWO issues:

### 1. Chat ID Format Mismatch

### Chat ID Formats

1. **Telegram Bot API** (used by running bot):
   - Supergroups: `-100{group_id}` (e.g., `-1001750074031`)
   - Private chats: positive numbers
   
2. **Telegram Desktop JSON Export** (used during import):
   - Supergroups: positive `{group_id}` (e.g., `1750074031`)
   - Missing the `-100` prefix

### Symptom

When the bot performed RAG search with `chatID = -1001750074031`, it couldn't find messages that were imported with `chatID = 1750074031`, resulting in 0 search results from historical messages.

### 2. Missing User IDs

All imported messages had `user_id = 0` because the import script wasn't parsing the `from_id` field from the Telegram export JSON. The export contains `"from_id": "user123456789"` but the script was ignoring this field.

## Solutions

### 1. Fix Existing Data (One-time migrations)

#### Step 1: Fix Chat IDs

Run the SQL migration to normalize existing chat IDs:

```bash
# Using Supabase SQL Editor (recommended)
# Copy and paste the contents of:
# deployments/supabase/fix_chat_id.sql

# OR using psql:
psql $DATABASE_URL < deployments/supabase/fix_chat_id.sql
```

The SQL script will:
1. Show current state of chat_messages grouped by chat_id
2. Convert positive supergroup IDs to Bot API format: `chat_id = -100 || chat_id`
3. Verify the changes
4. Check for duplicates (should be none due to UNIQUE constraint)

#### Step 2: Fix User IDs

Run the SQL migration to restore user IDs where possible:

```bash
# Using Supabase SQL Editor (recommended)
# Copy and paste the contents of:
# deployments/supabase/fix_user_id.sql

# OR using psql:
psql $DATABASE_URL < deployments/supabase/fix_user_id.sql
```

The SQL script will:
1. Find messages with `user_id = 0`
2. Match them with existing messages by username
3. Update user_id based on the mapping
4. Report remaining messages that couldn't be matched (users who haven't used the bot since deployment)

### 2. Updated Import Script

The `import_telegram_export.go` script has been updated with two new functions:

#### Parse User IDs

```go
func parseUserID(fromID string) int64 {
    if fromID == "" {
        return 0
    }

    var userID int64
    
    // Try formats: "user123456789", "channel123456789", or just "123456789"
    if _, err := fmt.Sscanf(fromID, "user%d", &userID); err == nil {
        return userID
    }
    if _, err := fmt.Sscanf(fromID, "channel%d", &userID); err == nil {
        return -userID // Channels use negative IDs
    }
    if _, err := fmt.Sscanf(fromID, "%d", &userID); err == nil {
        return userID
    }

    return 0
}
```

#### Normalize Chat IDs

The script automatically converts Telegram Desktop export format to Bot API format during import:

```go
func normalizeChatID(chatID int64) int64 {
    // If already negative, assume it's already in correct format
    if chatID < 0 {
        return chatID
    }

    // If positive and looks like a supergroup ID (large number > 1 billion)
    // Convert to Bot API format: -100 prefix
    if chatID > 1000000000 {
        return -1000000000000 - chatID
    }

    // For private chats or small IDs, keep as is
    return chatID
}
```

### 3. Verification

After applying the fix:

1. **Check database**:
   ```sql
   SELECT 
       chat_id,
       COUNT(*) as message_count,
       COUNT(CASE WHEN indexed = TRUE THEN 1 END) as indexed_count
   FROM chat_messages
   GROUP BY chat_id
   ORDER BY chat_id;
   ```

   Expected: All supergroup messages should have negative chat_id (e.g., `-1001750074031`)

2. **Test RAG search**:
   - Mention the bot in the group chat
   - Ask a question related to historical messages
   - Verify that RAG search returns relevant historical context

3. **Check logs**:
   ```bash
   # Look for RAG search logs showing results count
   grep "RAG search completed" logs/bot.log
   ```

## Files Changed

1. `deployments/supabase/fix_chat_id.sql` - SQL migration to fix chat_id format
2. `deployments/supabase/fix_user_id.sql` - SQL migration to restore user_id
3. `scripts/import_telegram_export.go` - Updated to parse user_id and normalize chat_id during import
4. `scripts/fix_chat_ids.sh` - Helper script (documentation/instructions)
5. `CHAT_ID_FIX.md` - This documentation

## Technical Details

### Telegram ID Formats

From Telegram Bot API documentation:

**Chat IDs:**
- **Private chats**: Positive integer (user_id)
- **Groups**: Negative integer
- **Supergroups/Channels**: Negative integer with format `-100{group_id}`

**User IDs in Telegram Export:**
- Format: `"from_id": "user123456789"` or `"channel123456789"`
- Must extract numeric part and handle prefix

### Why This Happened

1. **Chat ID**: Telegram Desktop exports use a different ID format than the Bot API. The export JSON contains the raw supergroup ID without the `-100` prefix, while the Bot API always includes it.

2. **User ID**: The original import script wasn't parsing the `from_id` field, which is a string like `"user123456789"` rather than a numeric field.

### SQL Function Impact

The `search_similar_messages()` function filters by chat_id:

```sql
AND (target_chat_id IS NULL OR cm.chat_id = target_chat_id)
```

This exact match filter was preventing historical messages from being found when their chat_id didn't match the format used by the running bot.

## Prevention

Going forward:
- Always use `normalizeChatID()` and `parseUserID()` when importing from Telegram exports
- The updated import script handles both automatically
- Any new import code should use the same normalization logic

## Testing

To test the fix:

1. Apply both SQL migrations (chat_id and user_id) to fix existing data
2. Verify data in Supabase:
   ```sql
   SELECT chat_id, user_id, username, COUNT(*) 
   FROM chat_messages 
   GROUP BY chat_id, user_id, username 
   ORDER BY COUNT(*) DESC;
   ```
3. Mention the bot in your group chat
4. Ask a question like: "что обсуждали по поводу работы?"
5. Verify bot responds with context from historical messages showing correct usernames
6. Check logs show: `RAG search completed` with `results_count > 0`

## Rollback

If you need to rollback (not recommended):

```sql
-- Convert back to export format (loses -100 prefix)
UPDATE chat_messages
SET chat_id = ABS(chat_id + 1000000000000)
WHERE chat_id < -1000000000000;
```

Note: This will break RAG search for current messages, so only use for data recovery.

