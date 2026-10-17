import { env } from "@/lib/env";
import { readCookie, ApiError } from "@/lib/api";

/**
 * Resolve a server-relative media path (e.g. an avatar URL returned by the
 * API) into an absolute URL. Returns undefined for empty input.
 */
export function mediaUrl(path: string | null | undefined): string | undefined {
  if (!path) return undefined;
  if (path.startsWith("http://") || path.startsWith("https://")) return path;
  return `${env.apiBaseUrl}${path.startsWith("/") ? path : `/${path}`}`;
}

/**
 * Upload a file to /api/v1/media/attachments using XMLHttpRequest to report progress.
 */
export function uploadWithProgress<T>(
  file: File,
  onProgress?: (percent: number) => void,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const form = new FormData();
    form.append("file", file);

    if (onProgress && xhr.upload) {
      xhr.upload.addEventListener("progress", (event) => {
        if (event.lengthComputable && event.total > 0) {
          const percent = Math.min(100, Math.round((event.loaded / event.total) * 100));
          onProgress(percent);
        }
      });
    }

    xhr.addEventListener("load", () => {
      let data: Record<string, unknown> | null = null;
      try {
        data = JSON.parse(xhr.responseText) as Record<string, unknown>;
      } catch {
        // non-json response
      }

      if (xhr.status >= 200 && xhr.status < 300) {
        if (data && "data" in data) {
          resolve(data.data as T);
        } else {
          resolve(data as unknown as T);
        }
      } else {
        const errorInfo =
          data && typeof data.error === "object" && data.error !== null
            ? (data.error as { code?: string; message?: string; details?: unknown })
            : undefined;
        reject(
          new ApiError({
            status: xhr.status,
            code: errorInfo?.code ?? "UPLOAD_ERROR",
            message:
              errorInfo?.message ??
              (typeof data?.message === "string"
                ? data.message
                : `Upload failed with status ${xhr.status}`),
            details: errorInfo?.details,
          }),
        );
      }
    });

    xhr.addEventListener("error", () => {
      reject(
        new ApiError({
          status: 0,
          code: "NETWORK_ERROR",
          message: "Network error during attachment upload.",
        }),
      );
    });

    xhr.addEventListener("abort", () => {
      reject(new Error("Upload aborted"));
    });

    const url = `${env.apiBaseUrl}/api/v1/media/attachments`;
    xhr.open("POST", url);
    xhr.withCredentials = true;

    const csrf = readCookie(env.csrfCookieName);
    if (csrf) {
      xhr.setRequestHeader("X-CSRF-Token", csrf);
    }

    xhr.send(form);
  });
}
