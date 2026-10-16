/**
 * Contacts & presence contract types. Mirror the Go handlers in
 * `services/api/internal/contacts`.
 */

/** Public peer profile embedded in contact and request payloads. */
export interface UserBrief {
  id: string;
  username: string | null;
  display_name: string;
  avatar_url: string | null;
  status_message: string | null;
  online: boolean;
  last_seen_at: string | null;
}

/** One accepted friendship viewed from the caller's side. */
export interface Contact {
  user: UserBrief;
  connected_at: string;
}

export type RequestDirection = "incoming" | "outgoing";

export type ContactRequestStatus = "pending" | "accepted" | "blocked";

/** A friend request, always paired with the counterpart's profile. */
export interface ContactRequest {
  id: string;
  status: ContactRequestStatus;
  direction: RequestDirection;
  user: UserBrief;
  created_at: string;
  accepted_at?: string | null;
}
