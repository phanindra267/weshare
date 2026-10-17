import { z } from "zod";

export const authorSchema = z.object({
  id: z.string().uuid(),
  username: z.string().nullable().optional(),
  display_name: z.string().min(1),
  avatar_url: z.string().url().nullable().optional(),
});

export const mediaSchema = z.object({
  id: z.string().uuid(),
  url: z.string().url(),
  media_type: z.enum(["image", "video"]),
  position: z.number().int().min(0),
});

export const momentSchema = z.object({
  id: z.string().uuid(),
  author: authorSchema,
  body: z.string(),
  visibility: z.enum(["friends", "public", "private"]),
  media: z.array(mediaSchema),
  like_count: z.number().int().min(0),
  comment_count: z.number().int().min(0),
  liked_by_me: z.boolean(),
  created_at: z.string(),
});

export const commentSchema = z.object({
  id: z.string().uuid(),
  moment_id: z.string().uuid(),
  author: authorSchema,
  body: z.string(),
  created_at: z.string(),
});

export const momentPageSchema = z.object({
  items: z.array(momentSchema),
  next_cursor: z.string().nullable().optional(),
});

export type Author = z.infer<typeof authorSchema>;
export type Media = z.infer<typeof mediaSchema>;
export type Moment = z.infer<typeof momentSchema>;
export type Comment = z.infer<typeof commentSchema>;
export type MomentPage = z.infer<typeof momentPageSchema>;
