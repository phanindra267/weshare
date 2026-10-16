import Link from "next/link";
import { BackendStatus } from "@/components/layout/BackendStatus";
import { Button } from "@/components/ui/Button";

export default function Home() {
  return (
    <main id="main" className="mx-auto max-w-2xl px-6 py-16">
      <div className="flex items-center gap-3">
        <div
          aria-hidden="true"
          className="bg-brand-600 grid size-11 place-items-center rounded-xl text-xl font-bold text-white"
        >
          W
        </div>
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">WeShare</h1>
          <p className="text-ink-muted text-sm">
            Local-first messaging, social, and mini-app platform
          </p>
        </div>
      </div>

      <div className="mt-8 flex flex-wrap gap-3">
        <Link href="/register">
          <Button>Create account</Button>
        </Link>
        <Link href="/login">
          <Button variant="secondary">Sign in</Button>
        </Link>
      </div>

      <section className="border-border-subtle bg-surface-raised mt-10 rounded-xl border p-6">
        <h2 className="text-base font-semibold">System status</h2>
        <p className="text-ink-muted mt-1 text-sm">
          This panel calls the real Go API at{" "}
          <code className="font-mono text-xs">/healthz</code> and{" "}
          <code className="font-mono text-xs">/readyz</code>.
        </p>
        <div className="mt-4">
          <BackendStatus />
        </div>
      </section>

      <p className="text-ink-muted mt-8 text-xs">
        Phase 1: authentication, accounts, and profiles. See{" "}
        <code className="font-mono">docs/PHASE_STATUS.md</code> for
        implementation progress.
      </p>
    </main>
  );
}
