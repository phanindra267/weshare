import type { Metadata } from "next";
import { RequireAuth } from "@/features/auth/RequireAuth";
import { ConversationsView } from "@/features/messaging/ConversationsView";

export const metadata: Metadata = {
  title: "Messages",
};

export default function MessagesPage() {
  return (
    <main id="main" className="mx-auto max-w-5xl px-6 py-10">
      <RequireAuth>
        <div className="flex flex-col gap-6">
          <h1 className="text-2xl font-semibold tracking-tight">Messages</h1>
          <ConversationsView />
        </div>
      </RequireAuth>
    </main>
  );
}
