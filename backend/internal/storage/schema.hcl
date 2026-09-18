# Desired database schema — source of truth for generated migrations.
# Regenerate SQL after edits with:
#   atlas migrate hash   (once, or after manual dir edits)
#   /opt/homebrew/bin/atlas migrate diff <name> \
#     --dir file://backend/internal/storage/migrations \
#     --dir-format goose \
#     --to file://backend/internal/storage/schema.hcl \
#     --dev-url "docker://postgres/16/dev?search_path=public"
# goose (embedded) applies the generated files at startup and in tests.

schema "public" {}

extension "citext" {
  schema = schema.public
}

table "users" {
  schema = schema.public
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "email" {
    type    = sql("citext")
    null    = false
  }
  column "password_hash" {
    type    = text
    null    = true
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  index "users_email_key" {
    unique  = true
    columns = [column.email]
  }
}

table "oauth_accounts" {
  schema = schema.public
  column "provider" {
    type = text
    null = false
  }
  column "provider_subject" {
    type = text
    null = false
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  primary_key {
    columns = [column.provider, column.provider_subject]
  }
  foreign_key "oauth_accounts_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "oauth_accounts_user_provider_key" {
    unique  = true
    columns = [column.user_id, column.provider]
  }
}

table "oauth_codes" {
  schema = schema.public
  column "code_hash" {
    type = text
    null = false
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "expires_at" {
    type = timestamptz
    null = false
  }
  primary_key {
    columns = [column.code_hash]
  }
  foreign_key "oauth_codes_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_oauth_codes_user" {
    columns = [column.user_id]
  }
}

table "refresh_tokens" {
  schema = schema.public
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "user_id" {
    type    = uuid
    null    = false
  }
  column "token_hash" {
    type    = text
    null    = false
  }
  column "expires_at" {
    type = timestamptz
    null = false
  }
  column "revoked_at" {
    type = timestamptz
    null = true
  }
  column "replaced_by" {
    type = text
    null = true
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "refresh_tokens_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "refresh_tokens_token_hash_key" {
    unique  = true
    columns = [column.token_hash]
  }
  index "idx_refresh_tokens_user" {
    columns = [column.user_id]
  }
}

table "conversations" {
  schema = schema.public
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "title" {
    type    = text
    null    = false
    default = ""
  }
  column "model" {
    type    = text
    null    = false
    default = ""
  }
  column "temperature" {
    type = real
    null = true
  }
  column "system_prompt" {
    type    = text
    null    = false
    default = ""
  }
  column "rag_enabled" {
    type    = boolean
    null    = false
    default = true
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  column "updated_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "conversations_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_conversations_user" {
    on {
      column = column.user_id
    }
    on {
      column = column.updated_at
      desc   = true
    }
  }
}

table "messages" {
  schema = schema.public
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "conversation_id" {
    type = uuid
    null = false
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "role" {
    type = text
    null = false
  }
  column "content" {
    type = text
    null = false
  }
  column "data" {
    type = jsonb
    null = false
  }
  column "input_tokens" {
    type    = integer
    null    = false
    default = 0
  }
  column "output_tokens" {
    type    = integer
    null    = false
    default = 0
  }
  column "requests" {
    type    = integer
    null    = false
    default = 0
  }
  column "cost_micro_usd" {
    type    = bigint
    null    = false
    default = 0
  }
  column "truncated" {
    type    = boolean
    null    = false
    default = false
  }
  column "model" {
    type    = text
    null    = false
    default = ""
  }
  column "sources" {
    type    = jsonb
    null    = false
    default = sql("'[]'::jsonb")
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "messages_conversation_id_fkey" {
    columns     = [column.conversation_id]
    ref_columns = [table.conversations.column.id]
    on_delete   = CASCADE
  }
  foreign_key "messages_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_messages_conversation" {
    columns = [column.conversation_id, column.created_at, column.id]
  }
}

table "usage_events" {
  schema = schema.public
  column "id" {
    type = bigserial
    null = false
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "kind" {
    type = text
    null = false
  }
  column "model" {
    type = text
    null = false
  }
  column "conversation_id" {
    type = uuid
    null = true
  }
  column "document_id" {
    type = uuid
    null = true
  }
  column "input_tokens" {
    type    = integer
    null    = false
    default = 0
  }
  column "output_tokens" {
    type    = integer
    null    = false
    default = 0
  }
  column "requests" {
    type    = integer
    null    = false
    default = 0
  }
  column "estimated" {
    type    = boolean
    null    = false
    default = false
  }
  column "cost_micro_usd" {
    type    = bigint
    null    = false
    default = 0
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "usage_events_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  foreign_key "usage_events_conversation_id_fkey" {
    columns     = [column.conversation_id]
    ref_columns = [table.conversations.column.id]
    on_delete   = SET_NULL
  }
  foreign_key "usage_events_document_id_fkey" {
    columns     = [column.document_id]
    ref_columns = [table.documents.column.id]
    on_delete   = SET_NULL
  }
  index "idx_usage_events_user_time" {
    columns = [column.user_id, column.created_at]
  }
}

table "tools" {
  schema = schema.public
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "name" {
    type = text
    null = false
  }
  column "description" {
    type = text
    null = false
  }
  column "method" {
    type = text
    null = false
  }
  column "url_template" {
    type = text
    null = false
  }
  column "params" {
    type    = jsonb
    null    = false
    default = "[]"
  }
  column "body_template" {
    type    = text
    null    = false
    default = ""
  }
  column "headers" {
    type    = jsonb
    null    = false
    default = "{}"
  }
  column "require_approval" {
    type    = boolean
    null    = false
    default = false
  }
  column "enabled" {
    type    = boolean
    null    = false
    default = true
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  column "updated_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "tools_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "tools_user_id_name_key" {
    unique  = true
    columns = [column.user_id, column.name]
  }
}

table "pending_tool_calls" {
  schema = schema.public
  # Surrogate PK: providers synthesize call IDs per request (Gemini emits
  # call-1, call-2, …), so call_id repeats across pauses of one conversation.
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "call_id" {
    type = text
    null = false
  }
  column "conversation_id" {
    type = uuid
    null = false
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "tool_name" {
    type = text
    null = false
  }
  column "args" {
    type = jsonb
    null = false
  }
  column "reason" {
    type    = text
    null    = false
    default = ""
  }
  column "status" {
    type    = text
    null    = false
    default = "pending"
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "pending_tool_calls_conversation_id_fkey" {
    columns     = [column.conversation_id]
    ref_columns = [table.conversations.column.id]
    on_delete   = CASCADE
  }
  foreign_key "pending_tool_calls_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_pending_tool_calls_conversation" {
    columns = [column.conversation_id, column.status]
  }
}

table "documents" {
  schema = schema.public
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "object_key" {
    type = text
    null = false
  }
  column "filename" {
    type = text
    null = false
  }
  column "mime" {
    type = text
    null = false
  }
  column "size_bytes" {
    type = bigint
    null = false
  }
  column "status" {
    type    = text
    null    = false
    default = "processing"
  }
  column "error" {
    type = text
    null = true
  }
  column "chunk_count" {
    type    = integer
    null    = false
    default = 0
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "documents_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_documents_user" {
    columns = [column.user_id, column.created_at]
  }
}
