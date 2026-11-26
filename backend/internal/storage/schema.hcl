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
  column "role" {
    type    = text
    null    = false
    default = "user"
  }
  column "default_instructions" {
    type    = text
    null    = false
    default = ""
  }
  column "default_top_p" {
    type = double
    null = true
  }
  column "default_max_output_tokens" {
    type    = integer
    null    = false
    default = 0
  }
  column "default_reasoning" {
    type    = text
    null    = false
    default = ""
  }
  column "email_verified_at" {
    type = timestamptz
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
  index "users_email_key" {
    unique  = true
    columns = [column.email]
  }
  index "idx_users_role" {
    columns = [column.role]
  }
}

table "user_provider_keys" {
  schema = schema.public
  column "user_id" {
    type = uuid
    null = false
  }
  column "provider" {
    type = text
    null = false
  }
  column "encrypted_key" {
    type = jsonb
    null = false
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
    columns = [column.user_id, column.provider]
  }
  foreign_key "user_provider_keys_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  check "user_provider_keys_provider_check" {
    expr = "provider IN ('gemini', 'openai', 'anthropic')"
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
  column "session_id" {
    type = uuid
    null = false
  }
  column "user_agent" {
    type    = text
    null    = false
    default = ""
  }
  column "ip" {
    type    = text
    null    = false
    default = ""
  }
  column "last_used_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
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
  index "idx_refresh_tokens_session" {
    columns = [column.user_id, column.session_id]
  }
}

table "auth_tokens" {
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
  column "token_hash" {
    type = text
    null = false
  }
  column "purpose" {
    type = text
    null = false
  }
  column "expires_at" {
    type = timestamptz
    null = false
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "auth_tokens_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "auth_tokens_token_hash_key" {
    unique  = true
    columns = [column.token_hash]
  }
  index "auth_tokens_user_purpose_key" {
    unique  = true
    columns = [column.user_id, column.purpose]
  }
  index "idx_auth_tokens_user_purpose" {
    columns = [column.user_id, column.purpose]
  }
  index "idx_auth_tokens_expiry" {
    columns = [column.expires_at]
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
  column "summary" {
    type    = text
    null    = false
    default = ""
  }
  column "summary_updated_at" {
    type = timestamptz
    null = true
  }
  column "summary_through_message_id" {
    type = uuid
    null = true
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
  column "bot_id" {
    type = uuid
    null = true
  }
  column "project_id" {
    type = uuid
    null = true
  }
  column "active_leaf_id" {
    type = uuid
    null = true
  }
  column "pinned_at" {
    type = timestamptz
    null = true
  }
  column "archived_at" {
    type = timestamptz
    null = true
  }
  column "title_source" {
    type    = text
    null    = false
    default = "heuristic"
  }
  column "top_p" {
    type = double
    null = true
  }
  column "max_output_tokens" {
    type    = integer
    null    = false
    default = 0
  }
  column "reasoning" {
    type    = text
    null    = false
    default = ""
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "conversations_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  foreign_key "conversations_bot_id_fkey" {
    columns     = [column.bot_id]
    ref_columns = [table.bots.column.id]
    on_delete   = SET_NULL
  }
  foreign_key "conversations_project_id_fkey" {
    columns     = [column.project_id]
    ref_columns = [table.projects.column.id]
    on_delete   = CASCADE
  }
  foreign_key "conversations_active_leaf_id_fkey" {
    columns     = [column.active_leaf_id]
    ref_columns = [table.messages.column.id]
    on_delete   = SET_NULL
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
  index "idx_conversations_bot" {
    columns = [column.bot_id]
  }
  index "idx_conversations_project" {
    columns = [column.project_id]
  }
  index "idx_conversations_pinned" {
    on {
      column = column.user_id
    }
    on {
      column = column.pinned_at
      desc   = true
    }
    where = sql("pinned_at IS NOT NULL")
  }
  index "idx_conversations_archived" {
    on {
      column = column.user_id
    }
    on {
      column = column.archived_at
      desc   = true
    }
    where = sql("archived_at IS NOT NULL")
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
  column "parent_id" {
    type = uuid
    null = true
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
  column "reply_to_id" {
    type = uuid
    null = true
  }
  column "quote_text" {
    type = text
    null = true
  }
  column "client_message_id" {
    type = text
    null = true
  }
  # Maintained by the messages_search_vector trigger; truncated to 100,000
  # characters to keep the GIN entry bounded.
  column "search_vector" {
    type = tsvector
    null = false
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
  foreign_key "messages_parent_id_fkey" {
    columns     = [column.parent_id]
    ref_columns = [table.messages.column.id]
    on_delete   = CASCADE
  }
  foreign_key "messages_reply_to_id_fkey" {
    columns     = [column.reply_to_id]
    ref_columns = [table.messages.column.id]
    on_delete   = SET_NULL
  }
  index "idx_messages_conversation" {
    columns = [column.conversation_id, column.created_at, column.id]
  }
  index "idx_messages_parent" {
    columns = [column.parent_id]
  }
  index "idx_messages_reply_to" {
    columns = [column.reply_to_id]
    where   = sql("reply_to_id IS NOT NULL")
  }
  index "idx_messages_client_id" {
    unique  = true
    columns = [column.conversation_id, column.client_message_id]
    where   = sql("client_message_id IS NOT NULL")
  }
  index "idx_messages_search" {
    type    = GIN
    columns = [column.search_vector]
  }
}

table "message_feedback" {
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
  column "message_id" {
    type = uuid
    null = false
  }
  column "rating" {
    type = text
    null = false
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
  foreign_key "message_feedback_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  foreign_key "message_feedback_message_id_fkey" {
    columns     = [column.message_id]
    ref_columns = [table.messages.column.id]
    on_delete   = CASCADE
  }
  check "message_feedback_rating_check" {
    expr = "rating IN ('up', 'down')"
  }
  index "message_feedback_user_message_key" {
    unique  = true
    columns = [column.user_id, column.message_id]
  }
  index "idx_message_feedback_message" {
    columns = [column.message_id]
  }
}

table "model_catalog" {
  schema = schema.public
  column "id" {
    type = text
    null = false
  }
  column "provider" {
    type = text
    null = false
  }
  column "label" {
    type = text
    null = false
  }
  column "input_per_m" {
    type    = double
    null    = false
    default = 0
  }
  column "output_per_m" {
    type    = double
    null    = false
    default = 0
  }
  column "enabled" {
    type    = boolean
    null    = false
    default = true
  }
  column "capabilities" {
    type    = jsonb
    null    = false
    default = sql("'{}'::jsonb")
  }
  column "sort_order" {
    type    = integer
    null    = false
    default = 0
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
  check "model_catalog_provider_check" {
    expr = "provider IN ('gemini', 'openai', 'anthropic')"
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
  column "message_id" {
    type = uuid
    null = true
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
  foreign_key "pending_tool_calls_message_id_fkey" {
    columns     = [column.message_id]
    ref_columns = [table.messages.column.id]
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
  column "project_id" {
    type = uuid
    null = true
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
  foreign_key "documents_project_id_fkey" {
    columns     = [column.project_id]
    ref_columns = [table.projects.column.id]
    on_delete   = CASCADE
  }
  index "idx_documents_user" {
    columns = [column.user_id, column.created_at]
  }
  index "idx_documents_project" {
    columns = [column.project_id]
  }
}

table "conversation_shares" {
  schema = schema.public
  column "conversation_id" {
    type = uuid
    null = false
  }
  column "token_hash" {
    type = text
    null = false
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  primary_key {
    columns = [column.conversation_id]
  }
  foreign_key "conversation_shares_conversation_id_fkey" {
    columns     = [column.conversation_id]
    ref_columns = [table.conversations.column.id]
    on_delete   = CASCADE
  }
}

table "user_memories" {
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
  column "content" {
    type = text
    null = false
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
  foreign_key "user_memories_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_user_memories_user" {
    columns = [column.user_id, column.created_at]
  }
}

table "projects" {
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
    type    = text
    null    = false
    default = ""
  }
  column "instructions" {
    type    = text
    null    = false
    default = ""
  }
  column "default_top_p" {
    type = double
    null = true
  }
  column "default_max_output_tokens" {
    type    = integer
    null    = false
    default = 0
  }
  column "default_reasoning" {
    type    = text
    null    = false
    default = ""
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
  foreign_key "projects_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_projects_user" {
    on {
      column = column.user_id
    }
    on {
      column = column.updated_at
      desc   = true
    }
  }
}

table "workflows" {
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
    type    = text
    null    = false
    default = ""
  }
  column "trigger_type" {
    type    = text
    null    = false
    default = "manual"
  }
  column "webhook_slug" {
    type = text
    null = true
  }
  column "webhook_secret" {
    type    = text
    null    = false
    default = ""
  }
  column "nodes" {
    type    = jsonb
    null    = false
    default = sql("'[]'::jsonb")
  }
  column "edges" {
    type    = jsonb
    null    = false
    default = sql("'[]'::jsonb")
  }
  column "expose_as_tool" {
    type    = boolean
    null    = false
    default = false
  }
  column "tool_name" {
    type    = text
    null    = false
    default = ""
  }
  column "tool_description" {
    type    = text
    null    = false
    default = ""
  }
  column "tool_require_approval" {
    type    = boolean
    null    = false
    default = true
  }
  column "is_active" {
    type    = boolean
    null    = false
    default = false
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
  foreign_key "workflows_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "workflows_webhook_slug_key" {
    unique  = true
    columns = [column.webhook_slug]
  }
  index "idx_workflows_user" {
    on {
      column = column.user_id
    }
    on {
      column = column.updated_at
      desc   = true
    }
  }
}

table "workflow_runs" {
  schema = schema.public
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "workflow_id" {
    type = uuid
    null = false
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "status" {
    type    = text
    null    = false
    default = "pending"
  }
  column "trigger_source" {
    type    = text
    null    = false
    default = "manual"
  }
  column "input_data" {
    type    = jsonb
    null    = false
    default = sql("'{}'::jsonb")
  }
  column "output_data" {
    type    = jsonb
    null    = false
    default = sql("'{}'::jsonb")
  }
  column "node_results" {
    type    = jsonb
    null    = false
    default = sql("'{}'::jsonb")
  }
  column "error" {
    type = text
    null = true
  }
  column "duration_ms" {
    type    = bigint
    null    = false
    default = 0
  }
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  column "finished_at" {
    type = timestamptz
    null = true
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "workflow_runs_workflow_id_fkey" {
    columns     = [column.workflow_id]
    ref_columns = [table.workflows.column.id]
    on_delete   = CASCADE
  }
  foreign_key "workflow_runs_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_workflow_runs_workflow" {
    columns = [column.workflow_id, column.created_at]
  }
  index "idx_workflow_runs_user" {
    columns = [column.user_id, column.created_at]
  }
}

table "workflow_credentials" {
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
  column "type" {
    type = text
    null = false
  }
  column "provider" {
    type    = text
    null    = false
    default = ""
  }
  column "scopes" {
    type    = sql("text[]")
    null    = false
    default = sql("'{}'::text[]")
  }
  column "expires_at" {
    type = timestamptz
    null = true
  }
  column "data" {
    type    = jsonb
    null    = false
    default = sql("'{}'::jsonb")
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
  foreign_key "workflow_credentials_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "workflow_credentials_user_name_key" {
    unique  = true
    columns = [column.user_id, column.name]
  }
}

table "tool_audit_logs" {
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
  column "caller_type" {
    type = text
    null = false
  }
  column "caller_id" {
    type    = text
    null    = false
    default = ""
  }
  column "tool_name" {
    type = text
    null = false
  }
  column "action" {
    type    = text
    null    = false
    default = "execute"
  }
  column "input_summary" {
    type    = text
    null    = false
    default = ""
  }
  column "output_summary" {
    type    = text
    null    = false
    default = ""
  }
  column "status" {
    type    = text
    null    = false
    default = "success"
  }
  column "error" {
    type = text
    null = true
  }
  column "duration_ms" {
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
  foreign_key "tool_audit_logs_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_tool_audit_logs_user" {
    columns = [column.user_id, column.created_at]
  }
}

table "search_queries" {
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
  column "conversation_id" {
    type = uuid
    null = true
  }
  column "query" {
    type = text
    null = false
  }
  column "kind" {
    type    = text
    null    = false
    default = "web_search"
  }
  column "provider" {
    type    = text
    null    = false
    default = ""
  }
  column "results_count" {
    type    = integer
    null    = false
    default = 0
  }
  column "duration_ms" {
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
  foreign_key "search_queries_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  foreign_key "search_queries_conversation_id_fkey" {
    columns     = [column.conversation_id]
    ref_columns = [table.conversations.column.id]
    on_delete   = SET_NULL
  }
  index "idx_search_queries_user_time" {
    columns = [column.user_id, column.created_at]
  }
  index "idx_search_queries_user_kind" {
    columns = [column.user_id, column.kind, column.created_at]
  }
}

table "bots" {
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
  column "emoji" {
    type    = text
    null    = false
    default = "🤖"
  }
  column "color" {
    type    = text
    null    = false
    default = "sage"
  }
  column "instructions" {
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
  column "top_p" {
    type = double
    null = true
  }
  column "max_output_tokens" {
    type    = integer
    null    = false
    default = 0
  }
  column "reasoning" {
    type    = text
    null    = false
    default = ""
  }
  column "rag_enabled" {
    type    = boolean
    null    = false
    default = true
  }
  column "is_archived" {
    type    = boolean
    null    = false
    default = false
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
  foreign_key "bots_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_bots_user" {
    on {
      column = column.user_id
    }
    on {
      column = column.updated_at
      desc   = true
    }
  }
}

table "bot_memories" {
  schema = schema.public
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "bot_id" {
    type = uuid
    null = false
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "content" {
    type = text
    null = false
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
  foreign_key "bot_memories_bot_id_fkey" {
    columns     = [column.bot_id]
    ref_columns = [table.bots.column.id]
    on_delete   = CASCADE
  }
  foreign_key "bot_memories_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_bot_memories_bot" {
    on {
      column = column.bot_id
    }
    on {
      column = column.created_at
      desc   = true
    }
  }
}

table "bot_runs" {
  schema = schema.public
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "bot_id" {
    type = uuid
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
  column "status" {
    type    = text
    null    = false
    default = "queued"
  }
  column "task" {
    type    = text
    null    = false
    default = ""
  }
  column "trigger_source" {
    type    = text
    null    = false
    default = "manual"
  }
  column "error" {
    type = text
    null = true
  }
  column "activity" {
    type    = jsonb
    null    = false
    default = sql("'[]'::jsonb")
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
  column "created_at" {
    type    = timestamptz
    null    = false
    default = sql("now()")
  }
  column "started_at" {
    type = timestamptz
    null = true
  }
  column "finished_at" {
    type = timestamptz
    null = true
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "bot_runs_bot_id_fkey" {
    columns     = [column.bot_id]
    ref_columns = [table.bots.column.id]
    on_delete   = CASCADE
  }
  foreign_key "bot_runs_conversation_id_fkey" {
    columns     = [column.conversation_id]
    ref_columns = [table.conversations.column.id]
    on_delete   = CASCADE
  }
  foreign_key "bot_runs_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_bot_runs_bot" {
    on {
      column = column.bot_id
    }
    on {
      column = column.created_at
      desc   = true
    }
  }
  index "idx_bot_runs_active" {
    on {
      column = column.status
    }
    where = sql("status IN ('queued','running','waiting_approval')")
  }
}

table "bot_routines" {
  schema = schema.public
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "bot_id" {
    type = uuid
    null = false
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "name" {
    type = text
    null = false
  }
  column "schedule" {
    type = text
    null = false
  }
  column "timezone" {
    type    = text
    null    = false
    default = "UTC"
  }
  column "prompt" {
    type = text
    null = false
  }
  column "enabled" {
    type    = boolean
    null    = false
    default = true
  }
  column "last_run_at" {
    type = timestamptz
    null = true
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
  foreign_key "bot_routines_bot_id_fkey" {
    columns     = [column.bot_id]
    ref_columns = [table.bots.column.id]
    on_delete   = CASCADE
  }
  foreign_key "bot_routines_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_bot_routines_bot" {
    on {
      column = column.bot_id
    }
    on {
      column = column.created_at
      desc   = true
    }
  }
  index "idx_bot_routines_enabled" {
    on {
      column = column.enabled
    }
    where = sql("enabled = true")
  }
}

table "bot_files" {
  schema = schema.public
  column "id" {
    type    = uuid
    null    = false
    default = sql("gen_random_uuid()")
  }
  column "bot_id" {
    type = uuid
    null = false
  }
  column "user_id" {
    type = uuid
    null = false
  }
  column "path" {
    type = text
    null = false
  }
  column "content" {
    type    = text
    null    = false
    default = ""
  }
  column "size_bytes" {
    type    = bigint
    null    = false
    default = 0
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
  foreign_key "bot_files_bot_id_fkey" {
    columns     = [column.bot_id]
    ref_columns = [table.bots.column.id]
    on_delete   = CASCADE
  }
  foreign_key "bot_files_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "bot_files_bot_path_key" {
    unique  = true
    columns = [column.bot_id, column.path]
  }
}


table "mcp_servers" {
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
  column "url" {
    type = text
    null = false
  }
  column "transport" {
    type    = text
    null    = false
    default = "http"
  }
  column "enabled" {
    type    = boolean
    null    = false
    default = true
  }
  column "headers" {
    type    = jsonb
    null    = false
    default = sql("'{}'::jsonb")
  }
  column "app_id" {
    type = text
    null = true
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
  foreign_key "mcp_servers_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "mcp_servers_user_id_name_key" {
    unique  = true
    columns = [column.user_id, column.name]
  }
  index "idx_mcp_servers_user_enabled" {
    columns = [column.user_id, column.enabled]
  }
  index "idx_mcp_servers_user_app" {
    columns = [column.user_id, column.app_id]
  }
}

table "saved_prompts" {
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
    type = text
    null = false
  }
  column "body" {
    type = text
    null = false
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
  foreign_key "saved_prompts_user_id_fkey" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_saved_prompts_user" {
    on {
      column = column.user_id
    }
    on {
      column = column.updated_at
      desc   = true
    }
  }
}
