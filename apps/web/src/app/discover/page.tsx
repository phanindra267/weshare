import type { Metadata } from "next";
import { RequireAuth } from "@/features/auth/RequireAuth";
import { DiscoverView } from "@/features/discover/DiscoverView";

export const metadata: Metadata = {
  title: "Discover",
};

export default function DiscoverPage() {
  return (
    <main id="main" className="mx-auto max-w-2xl px-6 py-10">
      <RequireAuth>
        <div className="flex flex-col gap-6">
          <h1 className="text-2xl font-semibold tracking-tight">Discover</h1>
          <DiscoverView />
        </div>
      </RequireAuth>
    </main>
  );
}
