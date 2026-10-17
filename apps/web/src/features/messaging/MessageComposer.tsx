"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { Textarea } from "@/components/ui/Input";
import { formatDuration } from "@/features/messaging/format";

/** How long after the last keystroke to signal "stopped typing". */
const TYPING_IDLE_MS = 2000;

export interface MessageComposerProps {
  disabled?: boolean;
  /** True while an attachment upload is in flight. */
  uploading?: boolean;
  onSend: (text: string) => void;
  /** Called when the local typing state changes (true on first keystroke). */
  onTyping: (typing: boolean) => void;
  /** Hand a picked file to the caller for upload + send. */
  onAttach?: (file: File) => void;
  /** Hand a recorded voice note to the caller for upload + send. */
  onVoice?: (blob: Blob, durationMs: number) => void;
}

/** True when the browser can record audio via MediaRecorder + getUserMedia. */
function canRecord(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof MediaRecorder !== "undefined" &&
    typeof navigator !== "undefined" &&
    Boolean(navigator.mediaDevices?.getUserMedia)
  );
}

/**
 * Message input with attachments and voice notes. Enter sends, Shift+Enter
 * inserts a newline. Emits typing signals on the first keystroke and after an
 * idle pause.
 */
export function MessageComposer({
  disabled,
  uploading,
  onSend,
  onTyping,
  onAttach,
  onVoice,
}: MessageComposerProps) {
  const [value, setValue] = React.useState("");
  const [recording, setRecording] = React.useState(false);
  const [elapsedMs, setElapsedMs] = React.useState(0);
  const [recordingSupported] = React.useState(
    () => canRecord() && Boolean(onVoice),
  );

  const typingRef = React.useRef(false);
  const idleTimer = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  const fileInputRef = React.useRef<HTMLInputElement>(null);
  const recorderRef = React.useRef<MediaRecorder | null>(null);
  const chunksRef = React.useRef<Blob[]>([]);
  const streamRef = React.useRef<MediaStream | null>(null);
  const startedAtRef = React.useRef(0);
  const tickRef = React.useRef<ReturnType<typeof setInterval> | null>(null);
  const cancelledRef = React.useRef(false);

  React.useEffect(() => {
    return () => {
      if (idleTimer.current) clearTimeout(idleTimer.current);
      if (tickRef.current) clearInterval(tickRef.current);
      streamRef.current?.getTracks().forEach((track) => track.stop());
    };
  }, []);

  const busy = disabled || recording;

  const stopTyping = React.useCallback(() => {
    if (idleTimer.current) {
      clearTimeout(idleTimer.current);
      idleTimer.current = null;
    }
    if (typingRef.current) {
      typingRef.current = false;
      onTyping(false);
    }
  }, [onTyping]);

  const handleChange = (event: React.ChangeEvent<HTMLTextAreaElement>) => {
    setValue(event.target.value);
    if (!typingRef.current) {
      typingRef.current = true;
      onTyping(true);
    }
    if (idleTimer.current) clearTimeout(idleTimer.current);
    idleTimer.current = setTimeout(stopTyping, TYPING_IDLE_MS);
  };

  const submit = () => {
    const text = value.trim();
    if (!text || disabled) return;
    stopTyping();
    onSend(text);
    setValue("");
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      submit();
    }
  };

  const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (file && onAttach) onAttach(file);
  };

  const finalizeRecording = React.useCallback(
    (durationMs: number) => {
      const type = recorderRef.current?.mimeType || "audio/webm";
      const chunks = chunksRef.current;
      const cancelled = cancelledRef.current;
      chunksRef.current = [];
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
      recorderRef.current = null;
      if (tickRef.current) {
        clearInterval(tickRef.current);
        tickRef.current = null;
      }
      setRecording(false);
      setElapsedMs(0);
      if (cancelled || !onVoice || chunks.length === 0) return;
      onVoice(new Blob(chunks, { type }), durationMs);
    },
    [onVoice],
  );

  const startRecording = async () => {
    if (!onVoice) return;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const recorder = new MediaRecorder(stream);
      streamRef.current = stream;
      recorderRef.current = recorder;
      chunksRef.current = [];
      cancelledRef.current = false;
      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) chunksRef.current.push(event.data);
      };
      recorder.onstop = () => {
        const durationMs = Date.now() - startedAtRef.current;
        finalizeRecording(durationMs);
      };
      startedAtRef.current = Date.now();
      setElapsedMs(0);
      setRecording(true);
      recorder.start();
      tickRef.current = setInterval(() => {
        setElapsedMs(Date.now() - startedAtRef.current);
      }, 200);
    } catch {
      finalizeRecording(0);
    }
  };

  const stopRecording = (cancel: boolean) => {
    cancelledRef.current = cancel;
    const recorder = recorderRef.current;
    if (recorder && recorder.state !== "inactive") {
      recorder.stop();
    } else {
      finalizeRecording(Date.now() - startedAtRef.current);
    }
  };

  return (
    <form
      className="flex items-end gap-2"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      {onAttach ? (
        <>
          <input
            ref={fileInputRef}
            type="file"
            className="sr-only"
            onChange={handleFileChange}
            tabIndex={-1}
            aria-hidden="true"
          />
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={busy}
            loading={uploading}
            aria-label="Attach a file"
            title="Attach a file"
            onClick={() => fileInputRef.current?.click()}
          >
            <span aria-hidden="true">📎</span>
          </Button>
        </>
      ) : null}

      {recording ? (
        <div className="flex flex-1 items-center gap-2" aria-live="polite">
          <span className="text-ink-muted text-sm">
            Recording {formatDuration(elapsedMs / 1000)}…
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => stopRecording(true)}
          >
            Cancel
          </Button>
          <Button type="button" size="sm" onClick={() => stopRecording(false)}>
            Send
          </Button>
        </div>
      ) : (
        <>
          <Textarea
            aria-label="Message"
            placeholder="Type a message"
            rows={1}
            value={value}
            disabled={busy}
            onChange={handleChange}
            onKeyDown={handleKeyDown}
            onBlur={stopTyping}
            className="max-h-40 min-h-10 resize-none"
          />
          {recordingSupported ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={disabled}
              aria-label="Record a voice message"
              title="Record a voice message"
              onClick={startRecording}
            >
              <span aria-hidden="true">🎤</span>
            </Button>
          ) : null}
          <Button type="submit" disabled={disabled || value.trim() === ""}>
            Send
          </Button>
        </>
      )}
    </form>
  );
}
