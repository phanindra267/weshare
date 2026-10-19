import type { Metadata } from "next";
import { RequireAuth } from "@/features/auth/RequireAuth";
import { SearchView } from "@/features/search/SearchView";

export const metadata: Metadata = {
  title: "Search",
};

export default function SearchPage() {
  return (
    <main id="main" className="mx-auto max-w-2xl px-6 py-10">
      <RequireAuth>
        <div className="flex flex-col gap-6">
          <h1 className="text-2xl font-semibold tracking-tight">Search</h1>
          <SearchView />
        </div>
      </RequireAuth>
    </main>
  );
}
