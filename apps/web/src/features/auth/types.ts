/**
 * Auth contract types. Mirror the Go handlers in
 * `services/api/internal/auth` and `services/api/internal/users`.
 */

export interface User {
  id: string;
  username: string | null;
  email: string;
  display_name: string;
  bio: string | null;
  avatar_url: string | null;
  status_message: string | null;
  online: boolean;
  last_seen_at: string | null;
  created_at: string;
}

export interface Session {
  id: string;
  user_agent?: string;
  ip?: string;
  last_used_at: string;
  expires_at: string;
  created_at: string;
  is_current: boolean;
}

export interface RegisterInput {
  email: string;
  username: string;
  password: string;
  display_name?: string;
}

export interface LoginInput {
  email: string;
  password: string;
}

/** Per-field validation errors keyed by field name. */
export type FieldErrors = Record<string, string>;
