import { api, ApiError } from "@/lib/api";
import type { Paginated } from "@/types/api";
import type {
  FieldErrors,
  LoginInput,
  RegisterInput,
  Session,
  User,
} from "@/features/auth/types";

/** Extract per-field validation errors from an ApiError, if present. */
export function fieldErrorsFrom(error: unknown): FieldErrors {
  if (error instanceof ApiError) {
    const details = error.details as { fields?: FieldErrors } | undefined;
    if (details?.fields) return details.fields;
  }
  return {};
}

export const authApi = {
  register: (input: RegisterInput) =>
    api.post<User>("/api/v1/auth/register", { json: input }),

  login: (input: LoginInput) =>
    api.post<User>("/api/v1/auth/login", { json: input }),

  logout: () => api.post<void>("/api/v1/auth/logout"),

  me: () => api.get<User>("/api/v1/auth/me"),

  changePassword: (currentPassword: string, newPassword: string) =>
    api.post<void>("/api/v1/auth/password", {
      json: { current_password: currentPassword, new_password: newPassword },
    }),

  listSessions: () => api.get<Paginated<Session>>("/api/v1/auth/sessions"),

  revokeSession: (id: string) => api.delete<void>(`/api/v1/auth/sessions/${id}`),

  revokeOtherSessions: () => api.delete<void>("/api/v1/auth/sessions"),
};
