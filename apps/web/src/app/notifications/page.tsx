import type { Metadata } from "next";
import { RequireAuth } from "@/features/auth/RequireAuth";
import { NotificationsView } from "@/features/notifications/NotificationsView";

export const metadata: Metadata = {
  title: "Notifications",
};

export default function NotificationsPage() {
  return (
    <main id="main" className="mx-auto max-w-2xl px-6 py-10">
      <RequireAuth>
        <div className="flex flex-col gap-6">
          <h1 className="text-2xl font-semibold tracking-tight">
            Notifications
          </h1>
          <NotificationsView />
        </div>
      </RequireAuth>
    </main>
  );
}
