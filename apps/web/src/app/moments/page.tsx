import type { Metadata } from "next";
import { RequireAuth } from "@/features/auth/RequireAuth";
import { MomentsView } from "@/features/moments/MomentsView";

export const metadata: Metadata = {
  title: "Moments",
};

export default function MomentsPage() {
  return (
    <main id="main" className="mx-auto max-w-2xl px-6 py-10">
      <RequireAuth>
        <div className="flex flex-col gap-6">
          <h1 className="text-2xl font-semibold tracking-tight">Moments</h1>
          <MomentsView />
        </div>
      </RequireAuth>
    </main>
  );
}
