import type { Metadata, Viewport } from "next";
import "./globals.css";
import { ToastProvider } from "@/components/ui/Toast";
import { AuthProvider } from "@/features/auth/AuthProvider";
import { AppHeader } from "@/components/layout/AppHeader";
import { PresenceHeartbeat } from "@/features/presence/PresenceHeartbeat";
import { RealtimeProvider } from "@/features/messaging/RealtimeProvider";
import { CallProvider } from "@/features/calls/CallProvider";
import { CallOverlay } from "@/features/calls/CallOverlay";

export const metadata: Metadata = {
  title: {
    default: "WeShare",
    template: "%s - WeShare",
  },
  description:
    "WeShare - a local-first communication, social, and mini-app platform.",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#ffffff" },
    { media: "(prefers-color-scheme: dark)", color: "#0f1419" },
  ],
};

/**
 * Applies the persisted/system theme before paint to avoid a flash of the
 * wrong theme. Kept tiny and inline on purpose.
 */
const themeInitScript = `(function(){try{var t=localStorage.getItem("weshare.theme");var d=window.matchMedia("(prefers-color-scheme: dark)").matches;var dark=t==="dark"||((!t||t==="system")&&d);document.documentElement.classList.toggle("dark",dark);}catch(e){}})();`;

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
      </head>
      <body>
        <a
          href="#main"
          className="focus:bg-surface-raised sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-100 focus:rounded focus:px-3 focus:py-2 focus:text-sm focus:shadow"
        >
          Skip to content
        </a>
        <ToastProvider>
          <AuthProvider>
            <RealtimeProvider>
              <CallProvider>
                <AppHeader />
                <PresenceHeartbeat />
                <CallOverlay />
                {children}
              </CallProvider>
            </RealtimeProvider>
          </AuthProvider>
        </ToastProvider>
      </body>
    </html>
  );
}
