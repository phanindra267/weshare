/**
 * Search contract types. Mirror the Go handlers in
 * `services/api/internal/search` and `services/api/internal/feed`
 * (`/search/posts`).
 */

import type { Message } from "@/features/messaging/types";
import type { Moment } from "@/features/moments/types";

/** Public profile of the other side of a direct conversation. */
export interface SearchPeer {
  id: string;
  username: string | null;
  display_name: string;
  avatar_url: string | null;
}

/** Conversation context attached to a message hit. */
export interface SearchConversation {
  id: string;
  kind: string;
  title: string | null;
  peer: SearchPeer | null;
}

/** One message-search result. */
export interface MessageSearchHit {
  message: Message;
  conversation: SearchConversation;
}

/** One profile-search result. */
export interface ProfileSearchHit {
  id: string;
  username: string | null;
  display_name: string;
  avatar_url: string | null;
  status_message: string | null;
  online: boolean;
  last_seen_at: string | null;
  is_contact: boolean;
}

/** One conversation (direct or group) search result. */
export interface ConversationSearchHit {
  id: string;
  kind: string;
  title: string | null;
  peer: SearchPeer | null;
  member_count: number;
  role: string;
  last_activity_at: string;
}

/** One post (moment) search result. Same shape as a feed Moment. */
export type PostSearchHit = Moment;
