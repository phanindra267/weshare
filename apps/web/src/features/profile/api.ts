import { api } from "@/lib/api";

export interface Profile {
  id: string;
  username: string | null;
  display_name: string;
  bio: string | null;
  avatar_url: string | null;
  status_message: string | null;
  online: boolean;
  last_seen_at: string | null;
  created_at: string;
}

export interface ProfileUpdate {
  display_name?: string;
  bio?: string;
  status_message?: string;
}

export const profileApi = {
  me: () => api.get<Profile>("/api/v1/me"),

  get: (id: string) => api.get<Profile>(`/api/v1/users/${id}`),

  update: (input: ProfileUpdate) =>
    api.patch<Profile>("/api/v1/me", { json: input }),

  uploadAvatar: (file: File) => {
    const formData = new FormData();
    formData.append("avatar", file);
    return api.post<Profile>("/api/v1/me/avatar", { formData });
  },
};
