"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
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
import { ForwardDialog } from "@/features/messaging/ForwardDialog";
import {
  applyPinned,
  applyReceipt,
  applyReactions,
  mergeMany,
  mergeMessage,
} from "@/features/messaging/thread";
import { conversationName, shortTime } from "@/features/messaging/format";
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

  // Advanced messaging (Phase 4): reply/edit/forward/delete/pin/search.
  const [replyTo, setReplyTo] = React.useState<ThreadMessage | null>(null);
  const [editing, setEditing] = React.useState<ThreadMessage | null>(null);
  const [forwardSource, setForwardSource] = React.useState<ThreadMessage | null>(
    null,
  );
  const [forwarding, setForwarding] = React.useState(false);
  const [deleteTarget, setDeleteTarget] = React.useState<ThreadMessage | null>(
    null,
  );
  const [deleting, setDeleting] = React.useState(false);
  const [pinsOpen, setPinsOpen] = React.useState(false);
  const [pins, setPins] = React.useState<Message[] | null>(null);
  const [pinsLoading, setPinsLoading] = React.useState(false);
  const [searchOpen, setSearchOpen] = React.useState(false);
  const [searchQuery, setSearchQuery] = React.useState("");
  const [searchResults, setSearchResults] = React.useState<Message[] | null>(
    null,
  );
  const [searchNext, setSearchNext] = React.useState<string | null>(null);
  const [searching, setSearching] = React.useState(false);
  const [highlightId, setHighlightId] = React.useState<string | null>(null);

  // Mirrors read inside stable callbacks so subscriptions never go stale.
  const selectedIdRef = React.useRef<string | null>(null);
  const messagesRef = React.useRef<ThreadMessage[]>([]);
  const currentUserIdRef = React.useRef(currentUserId);
  const infoOpenRef = React.useRef(infoOpen);
  const pinsOpenRef = React.useRef(pinsOpen);
  const replyToRef = React.useRef(replyTo);
  const editingRef = React.useRef(editing);
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
    pinsOpenRef.current = pinsOpen;
  }, [pinsOpen]);
  React.useEffect(() => {
    replyToRef.current = replyTo;
  }, [replyTo]);
  React.useEffect(() => {
    editingRef.current = editing;
  }, [editing]);
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

  /** Replace the sidebar preview when the message it shows is edited/deleted. */
  const patchConversationMessage = React.useCallback(
    (convId: string, message: Message) => {
      setConversations((prev) => {
        if (!prev) return prev;
        const index = prev.findIndex((c) => c.id === convId);
        if (index < 0 || prev[index].last_message?.id !== message.id) {
          return prev;
        }
        const next = [...prev];
        next[index] = { ...prev[index], last_message: message };
        return next;
      });
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
        case "message.updated": {
          if (event.conversation_id === selectedIdRef.current) {
            setMessages((prev) => mergeMessage(prev, event.message));
          }
          patchConversationMessage(event.conversation_id, event.message);
          break;
        }
        case "message.deleted": {
          if (event.conversation_id === selectedIdRef.current) {
            setMessages((prev) => mergeMessage(prev, event.message));
          }
          patchConversationMessage(event.conversation_id, event.message);
          break;
        }
        case "message.reaction": {
          if (event.conversation_id === selectedIdRef.current) {
            setMessages((prev) =>
              applyReactions(prev, event.message_id, event.reactions),
            );
          }
          break;
        }
        case "message.pinned": {
          if (event.conversation_id === selectedIdRef.current) {
            setMessages((prev) =>
              applyPinned(prev, event.message_id, event.pinned),
            );
            if (pinsOpenRef.current) {
              messagingApi.listPins(event.conversation_id).then(
                (items) => setPins(items),
                () => {
                  /* pinned list refresh is best-effort */
                },
              );
            }
          }
          break;
        }
        default:
          break;
      }
    },
    [
      updateConversationOnMessage,
      patchConversationMessage,
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
    // Phase 4 view state is per-thread.
    setReplyTo(null);
    setEditing(null);
    setSearchOpen(false);
    setSearchQuery("");
    setSearchResults(null);
    setSearchNext(null);
    setPinsOpen(false);
    setPins(null);
    setHighlightId(null);
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
    (convId: string, clientMsgId: string, text: string, replyToId?: string) => {
      const sentOverSocket = realtime.send({
        type: "message.send",
        request_id: clientMsgId,
        conversation_id: convId,
        client_msg_id: clientMsgId,
        message_type: "text",
        content: { text },
        ...(replyToId ? { reply_to_id: replyToId } : {}),
      });
      if (sentOverSocket) return;
      messagingApi
        .sendMessage(convId, {
          client_msg_id: clientMsgId,
          message_type: "text",
          content: { text },
          ...(replyToId ? { reply_to_id: replyToId } : {}),
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
      const reply = replyToRef.current;
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
        reply_to: reply
          ? {
              id: reply.id,
              sender_id: reply.sender_id,
              message_type: reply.message_type,
              content: reply.content,
              deleted: Boolean(reply.deleted_at),
              text: reply.content.text ?? "",
            }
          : null,
        forwarded_from: null,
        pinned_at: null,
        reactions: [],
        pending: true,
      };
      setMessages((prev) => [...prev, optimistic]);
      deliver(convId, clientMsgId, text, reply?.id);
      setReplyTo(null);
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

  // --- Phase 4 actions -------------------------------------------------------

  const loadPins = React.useCallback((convId: string) => {
    setPinsLoading(true);
    return messagingApi.listPins(convId).then(
      (items) => {
        setPins(items);
        setPinsLoading(false);
      },
      () => {
        setPins(null);
        setPinsLoading(false);
      },
    );
  }, []);
  const loadPinsRef = React.useRef(loadPins);
  React.useEffect(() => {
    loadPinsRef.current = loadPins;
  }, [loadPins]);

  const startReply = React.useCallback((message: ThreadMessage) => {
    setEditing(null);
    setReplyTo(message);
  }, []);

  const startEdit = React.useCallback((message: ThreadMessage) => {
    setReplyTo(null);
    setEditing(message);
  }, []);

  const cancelCompose = React.useCallback(() => {
    setReplyTo(null);
    setEditing(null);
  }, []);

  const submitEdit = React.useCallback(
    (text: string) => {
      const convId = selectedIdRef.current;
      const target = editingRef.current;
      if (!convId || !target) return;
      messagingApi.editMessage(convId, target.id, text).then(
        (updated) => {
          setMessages((prev) => mergeMessage(prev, updated));
          setEditing(null);
        },
        (cause: unknown) => {
          toast({
            title: "Could not edit message",
            description: cause instanceof ApiError ? cause.message : undefined,
            variant: "error",
          });
          setEditing(null);
        },
      );
    },
    [toast],
  );

  const doDelete = React.useCallback(() => {
    const convId = selectedIdRef.current;
    const target = deleteTarget;
    if (!convId || !target) return;
    setDeleting(true);
    messagingApi.deleteMessage(convId, target.id).then(
      (updated) => {
        setMessages((prev) => mergeMessage(prev, updated));
        setDeleteTarget(null);
        setDeleting(false);
      },
      (cause: unknown) => {
        toast({
          title: "Could not delete message",
          description: cause instanceof ApiError ? cause.message : undefined,
          variant: "error",
        });
        setDeleteTarget(null);
        setDeleting(false);
      },
    );
  }, [deleteTarget, toast]);

  const forwardTo = React.useCallback(
    (targetId: string) => {
      const source = forwardSource;
      if (!source) return;
      setForwarding(true);
      messagingApi
        .forwardMessage(targetId, {
          source_conversation_id: source.conversation_id,
          message_id: source.id,
        })
        .then(
          () => {
            if (targetId === selectedIdRef.current) {
              // The socket also delivers it; merging keeps the thread instant.
              void messagingApi
                .listMessages(targetId, { limit: 1 })
                .then((page) => {
                  const latest = page.items[0];
                  if (latest) setMessages((prev) => mergeMessage(prev, latest));
                })
                .catch(() => {
                  /* the socket event will deliver it */
                });
            }
            setForwardSource(null);
            toast({ title: "Message forwarded", variant: "success" });
          },
          (cause: unknown) => {
            toast({
              title: "Could not forward message",
              description: cause instanceof ApiError ? cause.message : undefined,
              variant: "error",
            });
          },
        )
        .finally(() => setForwarding(false));
    },
    [forwardSource, toast],
  );

  const toggleReact = React.useCallback(
    (message: ThreadMessage, emoji: string) => {
      const convId = selectedIdRef.current;
      if (!convId) return;
      const mine =
        message.reactions?.find((r) => r.emoji === emoji)?.mine ?? false;
      const request = mine
        ? messagingApi.removeReaction(convId, message.id, emoji)
        : messagingApi.addReaction(convId, message.id, emoji);
      request.then(
        (res) =>
          setMessages((prev) => applyReactions(prev, message.id, res.reactions)),
        (cause: unknown) => {
          toast({
            title: "Could not update reaction",
            description: cause instanceof ApiError ? cause.message : undefined,
            variant: "error",
          });
        },
      );
    },
    [toast],
  );

  const togglePin = React.useCallback(
    (message: ThreadMessage, pinned: boolean) => {
      const convId = selectedIdRef.current;
      if (!convId) return;
      const request = pinned
        ? messagingApi.pinMessage(convId, message.id)
        : messagingApi.unpinMessage(convId, message.id);
      request.then(
        () => {
          setMessages((prev) => applyPinned(prev, message.id, pinned));
          if (pinsOpenRef.current) void loadPinsRef.current(convId);
        },
        (cause: unknown) => {
          toast({
            title: pinned ? "Could not pin message" : "Could not unpin message",
            description: cause instanceof ApiError ? cause.message : undefined,
            variant: "error",
          });
        },
      );
    },
    [toast],
  );

  const togglePinsPanel = React.useCallback(() => {
    const convId = selectedIdRef.current;
    if (!convId) return;
    if (pinsOpenRef.current) {
      setPinsOpen(false);
      return;
    }
    setPinsOpen(true);
    void loadPinsRef.current(convId);
  }, []);

  const runSearch = React.useCallback(
    (event: React.FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      const convId = selectedIdRef.current;
      const query = searchQuery.trim();
      if (!convId || !query) return;
      setSearching(true);
      messagingApi.searchMessages(convId, query, { limit: 30 }).then(
        (page) => {
          setSearchResults(page.items);
          setSearchNext(page.next_cursor ?? null);
          setSearching(false);
        },
        (cause: unknown) => {
          setSearching(false);
          toast({
            title: "Search failed",
            description: cause instanceof ApiError ? cause.message : undefined,
            variant: "error",
          });
        },
      );
    },
    [searchQuery, toast],
  );

  const loadMoreSearch = React.useCallback(() => {
    const convId = selectedIdRef.current;
    const query = searchQuery.trim();
    const before = searchNext ? Number(searchNext) : 0;
    if (!convId || !query || !before) return;
    setSearching(true);
    messagingApi
      .searchMessages(convId, query, { limit: 30, before })
      .then(
        (page) => {
          setSearchResults((prev) => [...(prev ?? []), ...page.items]);
          setSearchNext(page.next_cursor ?? null);
          setSearching(false);
        },
        () => setSearching(false),
      );
  }, [searchQuery, searchNext]);

  const jumpToMessage = React.useCallback(
    (messageId: string) => {
      const element = document.getElementById(`message-${messageId}`);
      if (!element) {
        toast({
          title: "Message not loaded",
          description: "Load older messages to see it in the thread.",
          variant: "info",
        });
        return;
      }
      element.scrollIntoView({ block: "center" });
      setHighlightId(messageId);
      window.setTimeout(
        () => setHighlightId((current) => (current === messageId ? null : current)),
        2500,
      );
    },
    [toast],
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

  // Group owners/admins may moderate; in a direct chat both members may.
  const myRole =
    selected?.kind === "group" && members
      ? members.find((m) => m.id === currentUserId)?.role
      : undefined;
  const isGroupModerator = myRole === "owner" || myRole === "admin";
  const canPin = Boolean(
    selected && (selected.kind === "direct" || isGroupModerator),
  );
  const canDeleteOthers = Boolean(
    selected && selected.kind === "group" && isGroupModerator,
  );

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
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  aria-pressed={searchOpen}
                  onClick={() => setSearchOpen((open) => !open)}
                >
                  {searchOpen ? "Close search" : "Search"}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-pressed={pinsOpen}
                  onClick={togglePinsPanel}
                >
                  Pinned
                </Button>
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
              </div>
            </header>

            {searchOpen ? (
              <div className="border-border-subtle border-b p-3">
                <form onSubmit={runSearch} className="flex items-center gap-2">
                  <Input
                    id="message-search"
                    value={searchQuery}
                    placeholder="Search this conversation"
                    autoComplete="off"
                    aria-label="Search messages in this conversation"
                    onChange={(e) => setSearchQuery(e.target.value)}
                  />
                  <Button type="submit" size="sm" loading={searching}>
                    Search messages
                  </Button>
                </form>
                {searchResults === null ? null : searchResults.length === 0 ? (
                  <p className="text-ink-muted mt-2 text-xs">
                    No messages match “{searchQuery.trim()}”.
                  </p>
                ) : (
                  <ul className="mt-2 flex max-h-40 flex-col gap-1 overflow-y-auto">
                    {searchResults.map((result) => (
                      <li key={result.id}>
                        <button
                          type="button"
                          onClick={() => jumpToMessage(result.id)}
                          className="hover:bg-surface-muted w-full rounded px-2 py-1 text-left text-xs"
                        >
                          <span className="text-ink-muted">
                            {shortTime(result.created_at)}
                          </span>{" "}
                          <span className="text-ink">
                            {result.content.text || "(no text)"}
                          </span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
                {searchNext ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    loading={searching}
                    onClick={loadMoreSearch}
                  >
                    More results
                  </Button>
                ) : null}
              </div>
            ) : null}

            {pinsOpen ? (
              <div className="border-border-subtle border-b p-3">
                <p className="text-ink text-sm font-medium">Pinned messages</p>
                {pinsLoading ? (
                  <p className="text-ink-muted mt-1 text-xs">Loading…</p>
                ) : pins && pins.length > 0 ? (
                  <ul className="mt-1 flex max-h-40 flex-col gap-1 overflow-y-auto">
                    {pins.map((pin) => (
                      <li key={pin.id}>
                        <button
                          type="button"
                          onClick={() => jumpToMessage(pin.id)}
                          className="hover:bg-surface-muted w-full rounded px-2 py-1 text-left text-xs text-ink"
                        >
                          {pin.content.text || "(no text)"}
                        </button>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-ink-muted mt-1 text-xs">
                    Nothing pinned yet.
                  </p>
                )}
              </div>
            ) : null}

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
                canPin={canPin}
                canDeleteOthers={canDeleteOthers}
                highlightId={highlightId}
                onRetry={retryMessage}
                onReply={startReply}
                onEdit={startEdit}
                onDelete={(message) => setDeleteTarget(message)}
                onForward={(message) => setForwardSource(message)}
                onReact={toggleReact}
                onTogglePin={togglePin}
                onJumpTo={jumpToMessage}
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
              {replyTo || editing ? (
                <div className="mb-2 flex items-center justify-between gap-2 text-xs">
                  <span className="text-ink-muted truncate">
                    {editing
                      ? "Editing message"
                      : `Replying to ${
                          senderNames?.[replyTo?.sender_id ?? ""] ?? "message"
                        }`}
                  </span>
                  <Button variant="ghost" size="sm" onClick={cancelCompose}>
                    Cancel
                  </Button>
                </div>
              ) : null}
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
                key={editing ? `edit:${editing.id}` : "compose"}
                editing={Boolean(editing)}
                initialValue={editing?.content.text ?? ""}
                uploading={uploading}
                onSend={editing ? submitEdit : sendText}
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

      {forwardSource ? (
        <ForwardDialog
          open
          conversations={conversations}
          sourceId={forwardSource.conversation_id}
          busy={forwarding}
          onClose={() => setForwardSource(null)}
          onForward={forwardTo}
        />
      ) : null}

      {deleteTarget ? (
        <Dialog
          open
          onClose={() => setDeleteTarget(null)}
          title="Delete message"
          description="The message is removed for everyone in this conversation."
          footer={
            <>
              <Button
                variant="secondary"
                type="button"
                onClick={() => setDeleteTarget(null)}
              >
                Cancel
              </Button>
              <Button
                variant="danger"
                type="button"
                loading={deleting}
                onClick={doDelete}
              >
                Delete
              </Button>
            </>
          }
        >
          <p className="text-ink-muted text-sm">This cannot be undone.</p>
        </Dialog>
      ) : null}
    </div>
  );
}
