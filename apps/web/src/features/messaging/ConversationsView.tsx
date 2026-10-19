"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { EmptyState } from "@/components/ui/EmptyState";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { Skeleton } from "@/components/ui/Skeleton";
import { UploadProgress } from "@/components/ui/UploadProgress";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { useAuth } from "@/features/auth/AuthProvider";
import { fieldErrorsFrom } from "@/features/auth/api";
import type { FieldErrors } from "@/features/auth/types";
import { messagingApi } from "@/features/messaging/api";
import { useRealtime } from "@/features/messaging/RealtimeProvider";
import { ConversationList } from "@/features/messaging/ConversationList";
import { MessageList } from "@/features/messaging/MessageList";
import { MessageComposer } from "@/features/messaging/MessageComposer";
import { CreateGroupDialog } from "@/features/messaging/CreateGroupDialog";
import { GroupInfoDialog } from "@/features/messaging/GroupInfoDialog";
import { ConversationCallButtons } from "@/features/calls/ConversationCallButtons";
import { applyReceipt, mergeMany, mergeMessage } from "@/features/messaging/thread";
import { conversationName } from "@/features/messaging/format";
import type {
  Conversation,
  Member,
  Message,
  MessageContent,
  MessageType,
  ServerEvent,
  ThreadMessage,
} from "@/features/messaging/types";

const PAGE_SIZE = 50;
const MARK_READ_DEBOUNCE_MS = 400;
/** Auto-clear a peer's typing indicator if we miss the "stopped" event. */
const TYPING_EXPIRY_MS = 6000;

/**
 * Two-pane messaging UI: a conversation list and the selected thread.
 *
 * REST loads the authoritative history; the realtime socket delivers live
 * messages, receipts, typing, and presence. Sends are optimistic and reconciled
 * by `client_msg_id`, and a reconnect triggers a catch-up sync. Groups add a
 * creation flow, a member panel, and media/voice attachments.
 */
export function ConversationsView() {
  const { user } = useAuth();
  const realtime = useRealtime();
  const { toast } = useToast();
  const currentUserId = user?.id ?? "";

  const [conversations, setConversations] = React.useState<Conversation[] | null>(
    null,
  );
  const [listError, setListError] = React.useState<string | null>(null);

  const [selectedId, setSelectedId] = React.useState<string | null>(null);
  const [messages, setMessages] = React.useState<ThreadMessage[]>([]);
  const [threadLoading, setThreadLoading] = React.useState(false);
  const [threadError, setThreadError] = React.useState<string | null>(null);
  const [hasMore, setHasMore] = React.useState(false);
  const [loadingOlder, setLoadingOlder] = React.useState(false);

  const [typingConvIds, setTypingConvIds] = React.useState<Set<string>>(
    () => new Set(),
  );

  const [openId, setOpenId] = React.useState("");
  const [openFields, setOpenFields] = React.useState<FieldErrors>({});
  const [opening, setOpening] = React.useState(false);

  const [createOpen, setCreateOpen] = React.useState(false);
  const [infoOpen, setInfoOpen] = React.useState(false);
  const [members, setMembers] = React.useState<Member[] | null>(null);
  const [uploading, setUploading] = React.useState(false);
  const [uploadProgress, setUploadProgress] = React.useState(0);

  // Mirrors read inside stable callbacks so subscriptions never go stale.
  const selectedIdRef = React.useRef<string | null>(null);
  const messagesRef = React.useRef<ThreadMessage[]>([]);
  const currentUserIdRef = React.useRef(currentUserId);
  const infoOpenRef = React.useRef(infoOpen);
  const typingTimers = React.useRef(
    new Map<string, ReturnType<typeof setTimeout>>(),
  );
  const readTimer = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  // Multi-page catch-up bookkeeping: the last cursor we requested per
  // conversation, so a non-advancing cursor cannot spin forever.
  const syncAfterRef = React.useRef(new Map<string, number>());
  const sendEventRef = React.useRef(realtime.send);

  React.useEffect(() => {
    sendEventRef.current = realtime.send;
  }, [realtime]);

  React.useEffect(() => {
    selectedIdRef.current = selectedId;
  }, [selectedId]);
  React.useEffect(() => {
    messagesRef.current = messages;
  }, [messages]);
  React.useEffect(() => {
    currentUserIdRef.current = currentUserId;
  }, [currentUserId]);
  React.useEffect(() => {
    infoOpenRef.current = infoOpen;
  }, [infoOpen]);
  React.useEffect(() => {
    const timers = typingTimers.current;
    return () => {
      timers.forEach((timer) => clearTimeout(timer));
      if (readTimer.current) clearTimeout(readTimer.current);
    };
  }, []);

  const loadConversations = React.useCallback((): Promise<void> => {
    return messagingApi.listConversations({ limit: PAGE_SIZE }).then(
      (page) => {
        setConversations(page.items);
        setListError(null);
      },
      (cause: unknown) => {
        setListError(
          cause instanceof ApiError
            ? cause.message
            : "Unable to load your conversations.",
        );
      },
    );
  }, []);

  React.useEffect(() => {
    void loadConversations();
  }, [loadConversations]);

  const loadMembers = React.useCallback((convId: string): Promise<void> => {
    return messagingApi.listMembers(convId).then(
      (items) => setMembers(items),
      () => setMembers(null),
    );
  }, []);
  const loadMembersRef = React.useRef(loadMembers);
  React.useEffect(() => {
    loadMembersRef.current = loadMembers;
  }, [loadMembers]);
  const loadConversationsRef = React.useRef(loadConversations);
  React.useEffect(() => {
    loadConversationsRef.current = loadConversations;
  }, [loadConversations]);

  const sendRead = React.useCallback(
    (convId: string, sequence: number) => {
      if (sequence <= 0) return;
      const sent = realtime.send({
        type: "read",
        conversation_id: convId,
        sequence_no: sequence,
      });
      if (!sent) {
        void messagingApi.markRead(convId, sequence).catch(() => {
          /* read position is best-effort */
        });
      }
      setConversations((prev) =>
        prev
          ? prev.map((c) => (c.id === convId ? { ...c, unread_count: 0 } : c))
          : prev,
      );
    },
    [realtime],
  );
  const sendReadRef = React.useRef(sendRead);
  React.useEffect(() => {
    sendReadRef.current = sendRead;
  }, [sendRead]);

  const scheduleMarkRead = React.useCallback((convId: string) => {
    if (readTimer.current) clearTimeout(readTimer.current);
    readTimer.current = setTimeout(() => {
      readTimer.current = null;
      const sequence = messagesRef.current.reduce(
        (acc, m) => (m.pending ? acc : Math.max(acc, m.sequence_no)),
        0,
      );
      sendReadRef.current(convId, sequence);
    }, MARK_READ_DEBOUNCE_MS);
  }, []);

  const setTyping = React.useCallback((convId: string, on: boolean) => {
    const timers = typingTimers.current;
    const existing = timers.get(convId);
    if (existing) {
      clearTimeout(existing);
      timers.delete(convId);
    }
    setTypingConvIds((prev) => {
      const next = new Set(prev);
      if (on) next.add(convId);
      else next.delete(convId);
      return next;
    });
    if (on) {
      timers.set(
        convId,
        setTimeout(() => {
          timers.delete(convId);
          setTypingConvIds((prev) => {
            const next = new Set(prev);
            next.delete(convId);
            return next;
          });
        }, TYPING_EXPIRY_MS),
      );
    }
  }, []);

  const updateConversationOnMessage = React.useCallback(
    (convId: string, message: Message) => {
      setConversations((prev) => {
        if (!prev) return prev;
        const index = prev.findIndex((c) => c.id === convId);
        if (index < 0) return prev;
        const viewing = convId === selectedIdRef.current;
        const fromPeer = message.sender_id !== currentUserIdRef.current;
        const updated: Conversation = {
          ...prev[index],
          last_message: message,
          unread_count:
            viewing || !fromPeer
              ? prev[index].unread_count
              : prev[index].unread_count + 1,
        };
        return [updated, ...prev.filter((c) => c.id !== convId)];
      });
    },
    [],
  );

  const applyPresence = React.useCallback(
    (userId: string, online: boolean) => {
      setConversations((prev) =>
        prev
          ? prev.map((c) =>
              c.peer !== null && c.peer.id === userId
                ? { ...c, peer: { ...c.peer, online } }
                : c,
            )
          : prev,
      );
    },
    [],
  );

  const handleEvent = React.useCallback(
    (event: ServerEvent) => {
      switch (event.type) {
        case "message.created": {
          setMessages((prev) => mergeMessage(prev, event.message));
          updateConversationOnMessage(event.conversation_id, event.message);
          if (
            event.conversation_id === selectedIdRef.current &&
            event.message.sender_id !== currentUserIdRef.current
          ) {
            scheduleMarkRead(event.conversation_id);
          }
          break;
        }
        case "message.ack": {
          setMessages((prev) => mergeMessage(prev, event.message));
          updateConversationOnMessage(event.conversation_id, event.message);
          break;
        }
        case "message.receipt": {
          if (event.conversation_id === selectedIdRef.current) {
            setMessages((prev) =>
              applyReceipt(
                prev,
                currentUserIdRef.current,
                event.up_to_sequence,
                event.status,
              ),
            );
          }
          break;
        }
        case "typing": {
          if (event.user_id !== currentUserIdRef.current) {
            setTyping(event.conversation_id, event.typing);
          }
          break;
        }
        case "presence": {
          applyPresence(event.user_id, event.online);
          break;
        }
        case "sync.result": {
          const convId = event.conversation_id;
          if (convId === selectedIdRef.current) {
            setMessages((prev) => mergeMany(prev, event.messages));
          }
          // Multi-page catch-up: while the server reports more pages, request
          // the next one. The cursor advances monotonically; if it ever fails
          // to advance we stop, so a reconnect can never spin forever.
          const cursor = event.next_cursor;
          const after = cursor ? Number(cursor) : 0;
          if (!cursor || !Number.isFinite(after) || after <= 0) {
            syncAfterRef.current.delete(convId);
            break;
          }
          if (syncAfterRef.current.get(convId) === after) {
            syncAfterRef.current.delete(convId);
            break;
          }
          syncAfterRef.current.set(convId, after);
          sendEventRef.current({ type: "sync", conversation_id: convId, after });
          break;
        }
        case "conversation.updated": {
          setConversations((prev) =>
            prev
              ? prev.map((c) =>
                  c.id === event.conversation_id
                    ? {
                        ...c,
                        title: event.title,
                        avatar_url: event.avatar_url,
                        member_count: event.member_count,
                      }
                    : c,
                )
              : prev,
          );
          if (
            event.conversation_id === selectedIdRef.current &&
            infoOpenRef.current
          ) {
            void loadMembersRef.current(event.conversation_id);
          }
          break;
        }
        case "member.changed": {
          if (event.user_id === currentUserIdRef.current) {
            if (event.action === "removed") {
              // The caller was removed: drop the conversation from the list.
              setConversations((prev) =>
                prev ? prev.filter((c) => c.id !== event.conversation_id) : prev,
              );
              if (selectedIdRef.current === event.conversation_id) {
                setSelectedId(null);
                setMessages([]);
                setMembers(null);
              }
              break;
            }
            if (event.action === "added") {
              // The caller was just added to a new group; pull it into the list.
              void loadConversationsRef.current();
              break;
            }
          }
          if (event.conversation_id === selectedIdRef.current) {
            void loadMembersRef.current(event.conversation_id);
          }
          break;
        }
        default:
          break;
      }
    },
    [
      updateConversationOnMessage,
      scheduleMarkRead,
      setTyping,
      applyPresence,
    ],
  );

  React.useEffect(() => {
    return realtime.subscribe(handleEvent);
  }, [realtime, handleEvent]);

  // Reload history whenever the selected conversation changes. State is reset
  // by the selection handler, so updates here happen only in promise callbacks.
  React.useEffect(() => {
    const convId = selectedId;
    if (!convId) return;
    let cancelled = false;
    messagingApi.listMessages(convId, { limit: PAGE_SIZE }).then(
      (page) => {
        if (cancelled) return;
        setMessages(page.items);
        setHasMore(Boolean(page.next_cursor));
        setThreadLoading(false);
        const sequence = page.items.reduce(
          (acc, m) => Math.max(acc, m.sequence_no),
          0,
        );
        sendReadRef.current(convId, sequence);
      },
      (cause: unknown) => {
        if (cancelled) return;
        setThreadError(
          cause instanceof ApiError
            ? cause.message
            : "Unable to load messages.",
        );
        setThreadLoading(false);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [selectedId]);

  // Load members for group threads so senders can be labelled and the info
  // dialog has data ready. Keyed on the kind (a primitive) so ordinary message
  // traffic does not trigger a member refetch.
  const selectedKind =
    conversations?.find((c) => c.id === selectedId)?.kind ?? null;
  React.useEffect(() => {
    if (!selectedId || selectedKind !== "group") return;
    void loadMembers(selectedId);
  }, [selectedId, selectedKind, loadMembers]);

  // After a reconnect, refresh the list and sync the open thread.
  const { connectionId, send: sendEvent } = realtime;
  const connectedOnce = React.useRef(false);
  React.useEffect(() => {
    if (connectionId === 0) return;
    if (!connectedOnce.current) {
      connectedOnce.current = true;
      return;
    }
    void loadConversations();
    const convId = selectedIdRef.current;
    if (!convId) return;
    const after = messagesRef.current.reduce(
      (acc, m) => (m.pending ? acc : Math.max(acc, m.sequence_no)),
      0,
    );
    // Fresh catch-up for this thread: forget any previous cursor.
    syncAfterRef.current.delete(convId);
    sendEvent({ type: "sync", conversation_id: convId, after });
  }, [connectionId, sendEvent, loadConversations]);

  const selectConversation = React.useCallback((id: string) => {
    setSelectedId(id);
    setMessages([]);
    setThreadLoading(true);
    setThreadError(null);
    setHasMore(false);
    setMembers(null);
    // Opening a thread is a fresh view of it; drop any catch-up cursor.
    syncAfterRef.current.delete(id);
  }, []);

  const loadOlder = React.useCallback(() => {
    const convId = selectedIdRef.current;
    const oldest = messagesRef.current.find((m) => !m.pending);
    if (!convId || !oldest) return;
    setLoadingOlder(true);
    messagingApi
      .listMessages(convId, { limit: PAGE_SIZE, before: oldest.sequence_no })
      .then(
        (page) => {
          setMessages((prev) => mergeMany(prev, page.items));
          setHasMore(Boolean(page.next_cursor));
          setLoadingOlder(false);
        },
        () => setLoadingOlder(false),
      );
  }, []);

  const deliver = React.useCallback(
    (convId: string, clientMsgId: string, text: string) => {
      const sentOverSocket = realtime.send({
        type: "message.send",
        request_id: clientMsgId,
        conversation_id: convId,
        client_msg_id: clientMsgId,
        message_type: "text",
        content: { text },
      });
      if (sentOverSocket) return;
      messagingApi
        .sendMessage(convId, {
          client_msg_id: clientMsgId,
          message_type: "text",
          content: { text },
        })
        .then((message) => setMessages((prev) => mergeMessage(prev, message)))
        .catch(() => {
          setMessages((prev) =>
            prev.map((m) =>
              m.client_msg_id === clientMsgId
                ? { ...m, pending: false, failed: true }
                : m,
            ),
          );
        });
    },
    [realtime],
  );

  const sendText = React.useCallback(
    (text: string) => {
      const convId = selectedIdRef.current;
      if (!convId) return;
      const clientMsgId = crypto.randomUUID();
      const optimistic: ThreadMessage = {
        id: `pending:${clientMsgId}`,
        conversation_id: convId,
        sender_id: currentUserIdRef.current,
        sequence_no: Number.MAX_SAFE_INTEGER,
        client_msg_id: clientMsgId,
        message_type: "text",
        content: { text },
        status: "sent",
        created_at: new Date().toISOString(),
        edited_at: null,
        deleted_at: null,
        pending: true,
      };
      setMessages((prev) => [...prev, optimistic]);
      deliver(convId, clientMsgId, text);
    },
    [deliver],
  );

  const retryMessage = React.useCallback(
    (message: ThreadMessage) => {
      const clientMsgId = message.client_msg_id;
      if (!clientMsgId) return;
      setMessages((prev) =>
        prev.map((m) =>
          m.client_msg_id === clientMsgId
            ? { ...m, pending: true, failed: false }
            : m,
        ),
      );
      deliver(message.conversation_id, clientMsgId, message.content.text ?? "");
    },
    [deliver],
  );

  /** Upload a media file then send a message referencing it. */
  const uploadAndSend = React.useCallback(
    async (
      convId: string,
      file: File,
      mediaType: MessageType,
      duration?: number,
    ) => {
      setUploading(true);
      setUploadProgress(0);
      try {
        const attachment = await messagingApi.uploadAttachment(file, (percent) => {
          setUploadProgress(percent);
        });
        const content: MessageContent =
          mediaType === "audio"
            ? { url: attachment.url, duration }
            : mediaType === "image"
              ? { url: attachment.url }
              : mediaType === "video"
                ? { url: attachment.url, content_type: attachment.content_type }
                : { url: attachment.url, name: attachment.name, size: attachment.size };
        const clientMsgId = crypto.randomUUID();
        const message = await messagingApi.sendMessage(convId, {
          client_msg_id: clientMsgId,
          message_type: mediaType,
          content,
        });
        setMessages((prev) => mergeMessage(prev, message));
        updateConversationOnMessage(convId, message);
      } catch (cause) {
        toast({
          title: "Could not send attachment",
          description: cause instanceof ApiError ? cause.message : undefined,
          variant: "error",
        });
      } finally {
        setUploading(false);
        setUploadProgress(0);
      }
    },
    [toast, updateConversationOnMessage],
  );

  const handleAttach = React.useCallback(
    (file: File) => {
      const convId = selectedIdRef.current;
      if (!convId) return;
      const type: MessageType = file.type.startsWith("image/")
        ? "image"
        : file.type.startsWith("video/")
          ? "video"
          : file.type.startsWith("audio/")
            ? "audio"
            : "file";
      void uploadAndSend(convId, file, type);
    },
    [uploadAndSend],
  );

  const handleAttachError = React.useCallback(
    (message: string) => {
      toast({
        title: "Attachment rejected",
        description: message,
        variant: "error",
      });
    },
    [toast],
  );

  const handleVoice = React.useCallback(
    (blob: Blob, durationMs: number) => {
      const convId = selectedIdRef.current;
      if (!convId) return;
      const file = new File([blob], "voice-message.webm", {
        type: blob.type || "audio/webm",
      });
      void uploadAndSend(convId, file, "audio", Math.max(1, Math.round(durationMs / 1000)));
    },
    [uploadAndSend],
  );

  const sendTyping = React.useCallback(
    (on: boolean) => {
      const convId = selectedIdRef.current;
      if (!convId) return;
      realtime.send({
        type: "typing",
        conversation_id: convId,
        typing: on,
      });
    },
    [realtime],
  );

  const openConversation = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setOpenFields({});
    setOpening(true);
    try {
      const conversation = await messagingApi.openConversation(openId.trim());
      setConversations((prev) =>
        prev
          ? [conversation, ...prev.filter((c) => c.id !== conversation.id)]
          : [conversation],
      );
      setOpenId("");
      selectConversation(conversation.id);
    } catch (cause) {
      const fields = fieldErrorsFrom(cause);
      if (Object.keys(fields).length > 0) {
        setOpenFields(fields);
      } else {
        toast({
          title: "Could not start conversation",
          description: cause instanceof ApiError ? cause.message : undefined,
          variant: "error",
        });
      }
    } finally {
      setOpening(false);
    }
  };

  if (listError && !conversations) {
    return (
      <ErrorMessage
        title="Could not load conversations"
        message={listError}
        onRetry={() => {
          setListError(null);
          void loadConversations();
        }}
      />
    );
  }

  if (!conversations) {
    return (
      <div
        className="grid gap-4 md:grid-cols-[20rem_1fr]"
        aria-busy="true"
        aria-label="Loading conversations"
      >
        <Skeleton className="h-96 w-full" />
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }

  const selected = conversations.find((c) => c.id === selectedId) ?? null;
  const peerTyping = selected ? typingConvIds.has(selected.id) : false;
  const senderNames =
    selected?.kind === "group" && members
      ? Object.fromEntries(members.map((m) => [m.id, m.display_name]))
      : undefined;

  return (
    <div className="grid gap-4 md:grid-cols-[20rem_1fr]">
      <aside className="border-border-subtle bg-surface-raised flex max-h-[70vh] flex-col rounded-xl border p-3">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-ink text-sm font-semibold">Chats</h2>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setCreateOpen(true)}
          >
            New group
          </Button>
        </div>
        <form onSubmit={openConversation} className="mb-3 flex flex-col gap-1.5">
          <label
            htmlFor="open-user-id"
            className="text-ink text-sm font-medium"
          >
            Start a chat
          </label>
          <div className="flex items-center gap-2">
            <Input
              id="open-user-id"
              value={openId}
              placeholder="User id"
              autoComplete="off"
              invalid={Boolean(openFields.user_id)}
              onChange={(e) => setOpenId(e.target.value)}
            />
            <Button type="submit" size="sm" loading={opening}>
              Open
            </Button>
          </div>
          {openFields.user_id ? (
            <p role="alert" className="text-xs text-red-600">
              {openFields.user_id}
            </p>
          ) : null}
        </form>
        <div className="min-h-0 flex-1 overflow-y-auto">
          {conversations.length === 0 ? (
            <EmptyState
              title="No conversations"
              description="Start a chat with a contact, or create a group."
            />
          ) : (
            <ConversationList
              conversations={conversations}
              selectedId={selectedId}
              typingConvIds={typingConvIds}
              onSelect={selectConversation}
            />
          )}
        </div>
      </aside>

      <section className="border-border-subtle bg-surface-raised flex max-h-[70vh] flex-col rounded-xl border">
        {selected ? (
          <>
            <header className="border-border-subtle flex items-center justify-between gap-2 border-b p-3">
              <div className="flex items-center gap-2">
                <h2 className="text-ink text-sm font-semibold">
                  {conversationName(selected)}
                </h2>
                {selected.kind === "group" ? (
                  <span className="text-ink-muted text-xs">
                    {selected.member_count} member
                    {selected.member_count === 1 ? "" : "s"}
                  </span>
                ) : selected.peer?.online ? (
                  <span className="text-xs text-green-600">online</span>
                ) : (
                  <span className="text-ink-muted text-xs">offline</span>
                )}
              </div>
              <ConversationCallButtons conversation={selected} />
              {selected.kind === "group" ? (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setInfoOpen(true)}
                >
                  Group info
                </Button>
              ) : null}
            </header>

            {threadError ? (
              <div className="p-4">
                <ErrorMessage message={threadError} />
              </div>
            ) : threadLoading ? (
              <div className="flex-1 p-4">
                <Skeleton className="h-24 w-full" />
              </div>
            ) : (
              <MessageList
                messages={messages}
                currentUserId={currentUserId}
                typingName={
                  peerTyping
                    ? selected.kind === "group"
                      ? "Someone"
                      : (selected.peer?.display_name ?? "Someone")
                    : null
                }
                senderNames={senderNames}
                onRetry={retryMessage}
              />
            )}

            {hasMore ? (
              <div className="border-border-subtle border-t p-2 text-center">
                <Button
                  variant="ghost"
                  size="sm"
                  loading={loadingOlder}
                  onClick={loadOlder}
                >
                  Load older messages
                </Button>
              </div>
            ) : null}

            <div className="border-border-subtle border-t p-3">
              {uploading ? (
                <div className="mb-2">
                  <div className="mb-1 flex items-center justify-between text-xs text-ink-muted">
                    <span>Uploading attachment…</span>
                    <span>{uploadProgress}%</span>
                  </div>
                  <UploadProgress percent={uploadProgress} />
                </div>
              ) : null}
              <MessageComposer
                uploading={uploading}
                onSend={sendText}
                onTyping={sendTyping}
                onAttach={handleAttach}
                onAttachError={handleAttachError}
                onVoice={(blob, durationMs) =>
                  void handleVoice(blob, durationMs)
                }
              />
            </div>
          </>
        ) : (
          <div className="flex flex-1 items-center justify-center p-6">
            <EmptyState
              title="Select a conversation"
              description="Choose a chat from the list, or start a new one."
            />
          </div>
        )}
      </section>

      {createOpen ? (
        <CreateGroupDialog
          open
          onClose={() => setCreateOpen(false)}
          onCreated={(conversation) => {
            setConversations((prev) =>
              prev
                ? [conversation, ...prev.filter((c) => c.id !== conversation.id)]
                : [conversation],
            );
            selectConversation(conversation.id);
          }}
        />
      ) : null}

      {infoOpen && selected ? (
        <GroupInfoDialog
          open
          conversation={selected}
          currentUserId={currentUserId}
          onClose={() => setInfoOpen(false)}
          onChanged={() => void loadConversations()}
          onLeft={() => {
            setConversations((prev) =>
              prev ? prev.filter((c) => c.id !== selected.id) : prev,
            );
            setSelectedId(null);
            setMessages([]);
          }}
        />
      ) : null}
    </div>
  );
}
