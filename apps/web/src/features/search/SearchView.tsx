"use client";

import Link from "next/link";
import * as React from "react";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Skeleton } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { cn } from "@/lib/cn";
import { useAuth } from "@/features/auth/AuthProvider";
import { Avatar } from "@/features/messaging/Avatar";
import { conversationName, relativeTime, shortTime } from "@/features/messaging/format";
import { contactsApi } from "@/features/contacts/api";
import { MomentStack } from "@/features/moments/MomentStack";
import type { Moment } from "@/features/moments/types";
import { searchApi } from "@/features/search/api";
import type {
  ConversationSearchHit,
  MessageSearchHit,
  ProfileSearchHit,
} from "@/features/search/types";

type Tab = "all" | "messages" | "people" | "chats" | "posts";

const DEBOUNCE_MS = 300;
const MIN_QUERY = 1;
const PAGE_LIMIT = 20;
const ALL_LIMIT = 5;

const TABS: ReadonlyArray<[Tab, string]> = [
  ["all", "All"],
  ["messages", "Messages"],
  ["people", "People"],
  ["chats", "Chats"],
  ["posts", "Posts"],
];

function messageText(hit: MessageSearchHit): string {
  const content = hit.message.content as { text?: string } | null;
  return content?.text ?? "";
}

/**
 * Global search across the caller's messages, people, conversations/groups, and
 * posts. One query box, per-domain tabs, and an "All" tab that queries every
 * domain at once. Results are permission-scoped on the server.
 */
export function SearchView() {
  const { user } = useAuth();
  const { toast } = useToast();
  const currentUserId = user?.id ?? "";

  const [tab, setTab] = React.useState<Tab>("all");
  const [query, setQuery] = React.useState("");
  const trimmed = query.trim();

  const [loading, setLoading] = React.useState(false);
  const [loadingMore, setLoadingMore] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const [nextCursor, setNextCursor] = React.useState<string | null>(null);
  // Bumping a token lets "retry" re-run the search for the same query.
  const [reloadToken, setReloadToken] = React.useState(0);

  const [messages, setMessages] = React.useState<MessageSearchHit[]>([]);
  const [people, setPeople] = React.useState<ProfileSearchHit[]>([]);
  const [chats, setChats] = React.useState<ConversationSearchHit[]>([]);
  const [posts, setPosts] = React.useState<Moment[]>([]);
  const [addedIds, setAddedIds] = React.useState<Set<string>>(new Set());

  // Cursor per domain is a ref: "load more" reads it without making the search
  // effect depend on (and re-run for) every cursor change.
  const cursorRef = React.useRef<Partial<Record<Tab, string | null>>>({});
  // Monotonic request id so a slow response cannot overwrite a newer one.
  const reqIdRef = React.useRef(0);

  const run = React.useCallback(
    async (mode: "fresh" | "more") => {
      if (trimmed.length < MIN_QUERY) return;
      const myId = ++reqIdRef.current;
      if (mode === "more") setLoadingMore(true);
      else setLoading(true);

      try {
        if (tab === "all") {
          const [p, c, m, po] = await Promise.all([
            searchApi.users(trimmed, { limit: ALL_LIMIT }),
            searchApi.conversations(trimmed, { limit: ALL_LIMIT }),
            searchApi.messages(trimmed, { limit: ALL_LIMIT }),
            searchApi.posts(trimmed, { limit: ALL_LIMIT }),
          ]);
          if (myId !== reqIdRef.current) return;
          setPeople(p.items);
          setChats(c.items);
          setMessages(m.items);
          setPosts(po.items);
          setNextCursor(null);
          cursorRef.current = {};
        } else {
          const cursor = mode === "more" ? cursorRef.current[tab] : undefined;
          const params = { limit: PAGE_LIMIT, cursor: cursor ?? undefined };
          if (tab === "people") {
            const page = await searchApi.users(trimmed, params);
            if (myId !== reqIdRef.current) return;
            setPeople((prev) =>
              mode === "more" ? [...prev, ...page.items] : page.items,
            );
            cursorRef.current = { ...cursorRef.current, people: page.next_cursor };
            setNextCursor(page.next_cursor);
          } else if (tab === "chats") {
            const page = await searchApi.conversations(trimmed, params);
            if (myId !== reqIdRef.current) return;
            setChats((prev) =>
              mode === "more" ? [...prev, ...page.items] : page.items,
            );
            cursorRef.current = { ...cursorRef.current, chats: page.next_cursor };
            setNextCursor(page.next_cursor);
          } else if (tab === "messages") {
            const page = await searchApi.messages(trimmed, params);
            if (myId !== reqIdRef.current) return;
            setMessages((prev) =>
              mode === "more" ? [...prev, ...page.items] : page.items,
            );
            cursorRef.current = { ...cursorRef.current, messages: page.next_cursor };
            setNextCursor(page.next_cursor);
          } else {
            const page = await searchApi.posts(trimmed, params);
            if (myId !== reqIdRef.current) return;
            setPosts((prev) =>
              mode === "more" ? [...prev, ...page.items] : page.items,
            );
            cursorRef.current = { ...cursorRef.current, posts: page.next_cursor };
            setNextCursor(page.next_cursor);
          }
        }
        setError(null);
      } catch (err) {
        if (myId === reqIdRef.current) {
          setError(
            err instanceof ApiError ? err.message : "Search failed. Try again.",
          );
        }
      } finally {
        if (myId === reqIdRef.current) {
          setLoading(false);
          setLoadingMore(false);
        }
      }
    },
    [tab, trimmed],
  );

  React.useEffect(() => {
    const handle = setTimeout(() => {
      if (trimmed.length < MIN_QUERY) {
        // Invalidate any in-flight request and reset to the idle state.
        reqIdRef.current += 1;
        cursorRef.current = {};
        setLoading(false);
        setLoadingMore(false);
        setError(null);
        setNextCursor(null);
        setMessages([]);
        setPeople([]);
        setChats([]);
        setPosts([]);
        return;
      }
      void run("fresh");
    }, DEBOUNCE_MS);

    return () => clearTimeout(handle);
  }, [trimmed, tab, reloadToken, run]);

  async function addFriend(profile: ProfileSearchHit) {
    try {
      const result = await contactsApi.sendRequest(profile.id);
      setAddedIds((prev) => new Set(prev).add(profile.id));
      toast({
        title:
          result.status === "accepted"
            ? `You are now friends with ${profile.display_name}`
            : `Friend request sent to ${profile.display_name}`,
        variant: "success",
      });
    } catch (err) {
      toast({
        title: "Could not send request",
        description: err instanceof ApiError ? err.message : undefined,
        variant: "error",
      });
    }
  }

  const isIdle = trimmed.length < MIN_QUERY;

  function selectTab(next: Tab) {
    setTab(next);
  }

  const peopleSection = (
    <PeopleResults
      profiles={people}
      addedIds={addedIds}
      onAddFriend={addFriend}
      showSeeAll={tab === "all"}
      onSeeAll={() => selectTab("people")}
    />
  );
  const chatsSection = (
    <ChatResults chats={chats} showSeeAll={tab === "all"} onSeeAll={() => selectTab("chats")} />
  );
  const messagesSection = (
    <MessageResults
      hits={messages}
      showSeeAll={tab === "all"}
      onSeeAll={() => selectTab("messages")}
    />
  );
  const postsSection =
    posts.length > 0 ? (
      <section className="flex flex-col gap-2">
        {tab === "all" ? <SectionHeader title="Posts" onSeeAll={() => selectTab("posts")} /> : null}
        <MomentStack
          moments={posts}
          setMoments={setPosts}
          currentUserId={currentUserId}
        />
      </section>
    ) : (
      <EmptyState title="No posts found" />
    );

  return (
    <div className="flex flex-col gap-4">
      <Field label="Search" htmlFor="search-input">
        <Input
          id="search-input"
          type="search"
          value={query}
          placeholder="Search messages, people, chats, and posts"
          aria-label="Search"
          data-testid="search-input"
          onChange={(event) => setQuery(event.target.value)}
        />
      </Field>

      <div role="tablist" aria-label="Search categories" className="flex flex-wrap gap-2">
        {TABS.map(([value, label]) => (
          <button
            key={value}
            role="tab"
            type="button"
            aria-selected={tab === value}
            onClick={() => selectTab(value)}
            className={cn(
              "rounded-md px-3 py-1.5 text-sm font-medium",
              tab === value
                ? "bg-brand-600 text-white"
                : "text-ink-muted hover:text-ink",
            )}
          >
            {label}
          </button>
        ))}
      </div>

      {error ? (
        <ErrorMessage message={error} onRetry={() => setReloadToken((n) => n + 1)} />
      ) : null}

      {isIdle ? (
        <EmptyState
          title="Start typing to search"
          description="Find text in your messages and posts, and discover people, groups, and conversations."
        />
      ) : loading ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      ) : tab === "all" ? (
        <div className="flex flex-col gap-6">
          <section className="flex flex-col gap-2">
            <SectionHeader title="People" onSeeAll={() => selectTab("people")} />
            {people.length === 0 ? (
              <EmptyState title="No people found" />
            ) : (
              peopleSection
            )}
          </section>
          <section className="flex flex-col gap-2">
            <SectionHeader title="Chats" onSeeAll={() => selectTab("chats")} />
            {chats.length === 0 ? (
              <EmptyState title="No chats found" />
            ) : (
              chatsSection
            )}
          </section>
          <section className="flex flex-col gap-2">
            <SectionHeader title="Messages" onSeeAll={() => selectTab("messages")} />
            {messages.length === 0 ? (
              <EmptyState title="No messages found" />
            ) : (
              messagesSection
            )}
          </section>
          {postsSection}
        </div>
      ) : tab === "people" ? (
        people.length === 0 ? <EmptyState title="No people found" /> : peopleSection
      ) : tab === "chats" ? (
        chats.length === 0 ? <EmptyState title="No chats found" /> : chatsSection
      ) : tab === "messages" ? (
        messages.length === 0 ? <EmptyState title="No messages found" /> : messagesSection
      ) : postsSection}

      {!isIdle && !loading && tab !== "all" && nextCursor ? (
        <div className="flex justify-center">
          <Button
            variant="ghost"
            size="sm"
            loading={loadingMore}
            onClick={() => void run("more")}
          >
            Load more
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function SectionHeader({
  title,
  onSeeAll,
}: {
  title: string;
  onSeeAll: () => void;
}) {
  return (
    <div className="flex items-center justify-between">
      <h2 className="text-ink text-sm font-semibold">{title}</h2>
      <button
        type="button"
        onClick={onSeeAll}
        className="text-brand-600 text-xs font-medium hover:underline"
      >
        See all
      </button>
    </div>
  );
}

function ResultsList({
  testId,
  children,
}: {
  testId: string;
  children: React.ReactNode;
}) {
  return (
    <ul
      data-testid={testId}
      className="border-border-subtle bg-surface-raised overflow-hidden rounded-lg border"
    >
      {children}
    </ul>
  );
}

function PeopleResults({
  profiles,
  addedIds,
  onAddFriend,
  showSeeAll,
  onSeeAll,
}: {
  profiles: ProfileSearchHit[];
  addedIds: Set<string>;
  onAddFriend: (profile: ProfileSearchHit) => void;
  showSeeAll: boolean;
  onSeeAll: () => void;
}) {
  if (profiles.length === 0) return null;
  return (
    <section className="flex flex-col gap-2">
      {showSeeAll ? <SectionHeader title="People" onSeeAll={onSeeAll} /> : null}
      <ResultsList testId="people-results">
        {profiles.map((profile) => {
          const added = addedIds.has(profile.id);
          return (
            <li
              key={profile.id}
              className="border-border-subtle flex items-center gap-3 border-b px-3 py-3 last:border-b-0"
            >
              <Avatar
                name={profile.display_name}
                avatarUrl={profile.avatar_url}
                className="size-9"
              />
              <div className="min-w-0 flex-1">
                <p className="text-ink truncate text-sm font-medium">
                  {profile.display_name}
                </p>
                <p className="text-ink-muted truncate text-xs">
                  {profile.username ? `@${profile.username}` : ""}
                  {profile.status_message ? ` · ${profile.status_message}` : ""}
                </p>
              </div>
              {profile.is_contact || added ? (
                <Link href="/messages">
                  <Button variant="secondary" size="sm">
                    Message
                  </Button>
                </Link>
              ) : (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => onAddFriend(profile)}
                >
                  Add friend
                </Button>
              )}
            </li>
          );
        })}
      </ResultsList>
    </section>
  );
}

function ChatResults({
  chats,
  showSeeAll,
  onSeeAll,
}: {
  chats: ConversationSearchHit[];
  showSeeAll: boolean;
  onSeeAll: () => void;
}) {
  if (chats.length === 0) return null;
  return (
    <section className="flex flex-col gap-2">
      {showSeeAll ? <SectionHeader title="Chats" onSeeAll={onSeeAll} /> : null}
      <ResultsList testId="chat-results">
        {chats.map((chat) => (
          <li
            key={chat.id}
            className="border-border-subtle border-b last:border-b-0"
          >
            <Link
              href="/messages"
              className="flex items-center gap-3 px-3 py-3 hover:underline"
            >
              <Avatar
                name={conversationName(chat)}
                avatarUrl={chat.kind === "group" ? null : chat.peer?.avatar_url ?? null}
                className="size-9"
              />
              <div className="min-w-0 flex-1">
                <p className="text-ink truncate text-sm font-medium">
                  {conversationName(chat)}
                </p>
                <p className="text-ink-muted truncate text-xs">
                  {chat.kind === "group"
                    ? `Group · ${chat.member_count} member${chat.member_count === 1 ? "" : "s"}`
                    : chat.peer?.username
                      ? `@${chat.peer.username}`
                      : "Direct message"}
                </p>
              </div>
              <span className="text-ink-muted shrink-0 text-xs">
                {relativeTime(chat.last_activity_at)}
              </span>
            </Link>
          </li>
        ))}
      </ResultsList>
    </section>
  );
}

function MessageResults({
  hits,
  showSeeAll,
  onSeeAll,
}: {
  hits: MessageSearchHit[];
  showSeeAll: boolean;
  onSeeAll: () => void;
}) {
  if (hits.length === 0) return null;
  return (
    <section className="flex flex-col gap-2">
      {showSeeAll ? <SectionHeader title="Messages" onSeeAll={onSeeAll} /> : null}
      <ResultsList testId="message-results">
        {hits.map((hit) => (
          <li
            key={hit.message.id}
            className="border-border-subtle border-b last:border-b-0"
          >
            <Link href="/messages" className="block px-3 py-3 hover:underline">
              <div className="flex items-center justify-between gap-2">
                <span className="text-ink text-sm font-medium">
                  {conversationName(hit.conversation)}
                </span>
                <span className="text-ink-muted text-xs">
                  {shortTime(hit.message.created_at)}
                </span>
              </div>
              <p className="text-ink-muted mt-0.5 line-clamp-2 text-sm">
                {messageText(hit)}
              </p>
            </Link>
          </li>
        ))}
      </ResultsList>
    </section>
  );
}
