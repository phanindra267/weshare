import type { Metadata } from "next";
import { RequireAuth } from "@/features/auth/RequireAuth";
import { ProfileEditor } from "@/features/profile/ProfileEditor";
import { SessionsPanel } from "@/features/auth/SessionsPanel";
import { ChangePasswordForm } from "@/features/auth/ChangePasswordForm";

export const metadata: Metadata = {
  title: "Your profile",
};

function Section({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <section className="border-border-subtle bg-surface-raised rounded-xl border p-6">
      <h2 className="text-base font-semibold">{title}</h2>
      {description ? (
        <p className="text-ink-muted mt-1 text-sm">{description}</p>
      ) : null}
      <div className="mt-4">{children}</div>
    </section>
  );
}

export default function ProfilePage() {
  return (
    <main id="main" className="mx-auto max-w-2xl px-6 py-10">
      <RequireAuth>
        <div className="flex flex-col gap-6">
          <h1 className="text-2xl font-semibold tracking-tight">
            Your profile
          </h1>

          <Section
            title="Public profile"
            description="This information is visible to other users."
          >
            <ProfileEditor />
          </Section>

          <Section
            title="Active sessions"
            description="Devices currently signed in to your account."
          >
            <SessionsPanel />
          </Section>

          <Section
            title="Password"
            description="Changing your password signs out all other devices."
          >
            <ChangePasswordForm />
          </Section>
        </div>
      </RequireAuth>
    </main>
  );
}
