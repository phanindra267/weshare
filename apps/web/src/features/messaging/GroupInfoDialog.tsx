"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import { Input } from "@/components/ui/Input";
import { Skeleton } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { mediaUrl } from "@/lib/media";
import { messagingApi } from "@/features/messaging/api";
import { MemberPicker } from "@/features/messaging/MemberPicker";
import type { Conversation, Member, MemberRole } from "@/features/messaging/types";

export interface GroupInfoDialogProps {
  open: boolean;
  onClose: () => void;
  conversation: Conversation;
  currentUserId: string;
  /** Called after any change that affects the conversation list. */
  onChanged: () => void;
  /** Called after the caller leaves or is removed from the group. */
  onLeft: () => void;
}

const ROLE_RANK: Record<MemberRole, number> = { member: 0, admin: 1, owner: 2 };

function roleLabel(role: MemberRole): string {
  switch (role) {
    case "owner":
      return "Owner";
    case "admin":
      return "Admin";
    default:
      return "Member";
  }
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).slice(0, 2);
  return parts.map((p) => p[0]?.toUpperCase() ?? "").join("") || "?";
}

function MemberAvatar({ member }: { member: Member }) {
  const src = mediaUrl(member.avatar_url);
  return src ? (
    <img
      src={src}
      alt=""
      className="bg-surface-overlay size-9 shrink-0 rounded-full object-cover"
    />
  ) : (
    <span
      aria-hidden="true"
      className="bg-surface-overlay text-ink-muted grid size-9 shrink-0 place-items-center rounded-full text-xs font-medium"
    >
      {initials(member.display_name)}
    </span>
  );
}

/**
 * Group management panel: rename, add/remove members, promote/demote admins,
 * and leave. Every action is re-authorized by the backend; the UI only hides
 * controls the caller is known to be unable to use.
 */
export function GroupInfoDialog({
  open,
  onClose,
  conversation,
  currentUserId,
  onChanged,
  onLeft,
}: GroupInfoDialogProps) {
  const { toast } = useToast();
  const [members, setMembers] = React.useState<Member[] | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const [title, setTitle] = React.useState(conversation.title ?? "");
  const [renaming, setRenaming] = React.useState(false);
  const [busy, setBusy] = React.useState<string | null>(null);
  const [inviting, setInviting] = React.useState(false);
  const [inviteSelected, setInviteSelected] = React.useState<string[]>([]);

  const load = React.useCallback((): Promise<void> => {
    return messagingApi.listMembers(conversation.id).then(
      (items) => {
        setMembers(items);
        setError(null);
      },
      (cause: unknown) => {
        setError(
          cause instanceof ApiError
            ? cause.message
            : "Unable to load group members.",
        );
      },
    );
  }, [conversation.id]);

  // The dialog is mounted only while open, so its state starts fresh on each
  // open and the members simply stream in.
  React.useEffect(() => {
    void load();
  }, [load]);

  const myRole = conversation.role;
  const canManage = ROLE_RANK[myRole] >= ROLE_RANK.admin;
  const canPromote = myRole === "owner";

  const run = async (
    key: string,
    action: () => Promise<unknown>,
    successTitle: string,
    failureTitle: string,
  ) => {
    setBusy(key);
    try {
      await action();
      toast({ title: successTitle, variant: "success" });
      await load();
      onChanged();
    } catch (cause) {
      toast({
        title: failureTitle,
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setBusy(null);
    }
  };

  const rename = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const next = title.trim();
    if (!next || next === (conversation.title ?? "")) return;
    setRenaming(true);
    try {
      await messagingApi.updateGroup(conversation.id, { title: next });
      toast({ title: "Group renamed", variant: "success" });
      onChanged();
    } catch (cause) {
      toast({
        title: "Could not rename group",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setRenaming(false);
    }
  };

  const invite = async () => {
    if (inviteSelected.length === 0) return;
    await run(
      "invite",
      () => messagingApi.addMembers(conversation.id, inviteSelected),
      "Members added",
      "Could not add members",
    );
    setInviteSelected([]);
    setInviting(false);
  };

  const leave = async () => {
    setBusy("leave");
    try {
      await messagingApi.leaveGroup(conversation.id);
      toast({ title: "You left the group", variant: "success" });
      onLeft();
      onClose();
    } catch (cause) {
      toast({
        title: "Could not leave group",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setBusy(null);
    }
  };

  const memberIds = members?.map((m) => m.id) ?? [];

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Group info"
      description={`${conversation.member_count} member${
        conversation.member_count === 1 ? "" : "s"
      }`}
      footer={
        <Button variant="danger" loading={busy === "leave"} onClick={leave}>
          Leave group
        </Button>
      }
    >
      {canManage ? (
        <form onSubmit={rename} className="mb-4 flex items-end gap-2">
          <label htmlFor="group-rename" className="flex-1">
            <span className="text-ink mb-1 block text-sm font-medium">
              Group name
            </span>
            <Input
              id="group-rename"
              value={title}
              maxLength={64}
              onChange={(e) => setTitle(e.target.value)}
            />
          </label>
          <Button type="submit" size="sm" loading={renaming}>
            Save
          </Button>
        </form>
      ) : (
        <p className="text-ink mb-4 text-sm font-medium">
          {conversation.title ?? "Group"}
        </p>
      )}

      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-ink text-sm font-semibold">Members</h3>
        {canManage ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setInviting((v) => !v)}
            aria-expanded={inviting}
          >
            {inviting ? "Done" : "Add members"}
          </Button>
        ) : null}
      </div>

      {inviting ? (
        <div className="mb-4 space-y-2">
          <MemberPicker
            idPrefix={`invite-${conversation.id}`}
            excludeIds={memberIds}
            selected={inviteSelected}
            onChange={setInviteSelected}
          />
          <Button
            size="sm"
            loading={busy === "invite"}
            disabled={inviteSelected.length === 0}
            onClick={() => void invite()}
          >
            Add selected
          </Button>
        </div>
      ) : null}

      {error && !members ? (
        <p role="alert" className="text-xs text-red-600">
          {error}
        </p>
      ) : !members ? (
        <div className="space-y-2" aria-busy="true">
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      ) : (
        <ul className="flex flex-col gap-1">
          {members.map((member) => {
            const isMe = member.id === currentUserId;
            const targetRank = ROLE_RANK[member.role];
            const canRemove =
              canManage && !isMe && member.role !== "owner" &&
              ROLE_RANK[myRole] > targetRank;
            const canToggleRole = canPromote && !isMe && member.role !== "owner";
            return (
              <li
                key={member.id}
                className="flex items-center gap-3 rounded-lg p-2"
              >
                <MemberAvatar member={member} />
                <div className="min-w-0 flex-1">
                  <p className="text-ink truncate text-sm">
                    {member.display_name}
                    {isMe ? (
                      <span className="text-ink-muted"> (you)</span>
                    ) : null}
                  </p>
                  <p className="text-ink-muted text-xs">
                    {roleLabel(member.role)}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  {canToggleRole ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      loading={busy === `role:${member.id}`}
                      onClick={() =>
                        void run(
                          `role:${member.id}`,
                          () =>
                            messagingApi.setMemberRole(
                              conversation.id,
                              member.id,
                              member.role === "admin" ? "member" : "admin",
                            ),
                          member.role === "admin"
                            ? "Admin removed"
                            : "Made admin",
                          "Could not change role",
                        )
                      }
                    >
                      {member.role === "admin" ? "Demote" : "Make admin"}
                    </Button>
                  ) : null}
                  {canRemove ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      loading={busy === `remove:${member.id}`}
                      onClick={() =>
                        void run(
                          `remove:${member.id}`,
                          () =>
                            messagingApi.removeMember(
                              conversation.id,
                              member.id,
                            ),
                          "Member removed",
                          "Could not remove member",
                        )
                      }
                    >
                      Remove
                    </Button>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </Dialog>
  );
}
