import type { Metadata } from "next";
import { RequireAuth } from "@/features/auth/RequireAuth";
import { ContactsView } from "@/features/contacts/ContactsView";

export const metadata: Metadata = {
  title: "Contacts",
};

export default function ContactsPage() {
  return (
    <main id="main" className="mx-auto max-w-2xl px-6 py-10">
      <RequireAuth>
        <div className="flex flex-col gap-6">
          <h1 className="text-2xl font-semibold tracking-tight">Contacts</h1>
          <ContactsView />
        </div>
      </RequireAuth>
    </main>
  );
}
