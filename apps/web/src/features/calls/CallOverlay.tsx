"use client";

import * as React from "react";

import { Button } from "@/components/ui/Button";
import { useCall, type Peer } from "@/features/calls/CallProvider";

/**
 * Starts playback without letting a refusal (or a stubbed environment) throw.
 *
 * Browsers return a promise that rejects when autoplay is blocked; jsdom's
 * stub returns `undefined`. Both must be tolerated, since this runs in an
 * effect and an exception there would fail the render.
 */
function safePlay(element: HTMLMediaElement) {
  try {
    const attempt = element.play();
    if (attempt && typeof attempt.catch === "function") {
      void attempt.catch(() => {
        // Blocked autoplay; the stream is still connected to the element.
      });
    }
  } catch {
    // Detached element or a non-functional stub; nothing to start.
  }
}

/** Binds a stream to a `<video>` element, since React cannot attach them. */
function MediaVideo({
  stream,
  muted,
  className,
}: {
  stream: MediaStream;
  muted: boolean;
  className?: string;
}) {
  const ref = React.useRef<HTMLVideoElement | null>(null);

  React.useEffect(() => {
    const element = ref.current;
    if (!element) return;
    if (element.srcObject !== stream) element.srcObject = stream;
    // Muting the local preview keeps the attempt gesture-free.
    safePlay(element);
    return () => {
      element.srcObject = null;
    };
  }, [stream]);

  return (
    <video
      ref={ref}
      autoPlay
      playsInline
      muted={muted}
      className={className}
    />
  );
}

/**
 * Audio-only calls need a playback element too — without one the remote
 * stream is attached to nothing and the peer is heard only in theory.
 */
function MediaAudio({ stream }: { stream: MediaStream }) {
  const ref = React.useRef<HTMLAudioElement | null>(null);

  React.useEffect(() => {
    const element = ref.current;
    if (!element) return;
    if (element.srcObject !== stream) element.srcObject = stream;
    safePlay(element);
    return () => {
      element.srcObject = null;
    };
  }, [stream]);

  return <audio ref={ref} autoPlay />;
}

function PeerTile({ peer, kind }: { peer: Peer; kind: string | null }) {
  const hasStream = Boolean(peer.stream);
  return (
    <div className="bg-canvas relative aspect-video w-40 overflow-hidden rounded-lg border">
      {kind === "video" && hasStream && peer.stream ? (
        <MediaVideo
          stream={peer.stream}
          muted={false}
          className="h-full w-full object-cover"
        />
      ) : kind === "audio" && hasStream && peer.stream ? (
        <MediaAudio stream={peer.stream} />
      ) : null}
      <span className="text-ink absolute bottom-1 left-1 rounded bg-black/60 px-1 text-xs text-white">
        {peer.displayName}
        {peer.connected ? "" : " (connecting)"}
      </span>
    </div>
  );
}

export function CallOverlay() {
  const call = useCall();

  if (!call || call.state === "idle") return null;

  if (call.state === "incoming-ringing" && call.incoming) {
    return (
      <div
        className="fixed inset-x-0 bottom-6 z-50 flex justify-center"
        data-testid="call-incoming"
      >
        <div className="border-border-subtle bg-surface-raised flex items-center gap-4 rounded-2xl border p-4 shadow-lg">
          <div className="flex flex-col">
            <span className="font-medium">
              {call.incoming.from.display_name} is calling
            </span>
            <span className="text-ink-subtle text-sm">
              {call.incoming.kind}
              {call.incoming.group ? " (group)" : ""}
            </span>
          </div>
          <Button
            size="sm"
            onClick={call.acceptCall}
            data-testid="call-accept"
          >
            Accept
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={call.rejectCall}
            data-testid="call-reject"
          >
            Reject
          </Button>
        </div>
      </div>
    );
  }

  const counterparty =
    call.participants.map((p) => p.display_name).join(", ") || null;

  return (
    <div
      className="fixed inset-x-0 bottom-6 z-50 flex justify-center"
      data-testid="call-active"
    >
      <div className="border-border-subtle bg-surface-raised flex items-center gap-4 rounded-2xl border p-4 shadow-lg">
        <div className="flex flex-col">
          <span className="font-medium">
            {counterparty ? `In call with ${counterparty}` : "In call"}
          </span>
          <span className="text-ink-subtle text-sm" data-testid="call-state">
            {call.state}
          </span>
        </div>

        {call.localStream && call.kind === "video" ? (
          <MediaVideo
            stream={call.localStream}
            muted
            className="border-border-subtle h-16 w-24 rounded-md border object-cover"
          />
        ) : null}

        {call.remotePeers.length > 0 ? (
          <div className="flex gap-2" data-testid="remote-peers">
            {call.remotePeers.map((peer) => (
              <PeerTile key={peer.id} peer={peer} kind={call.kind} />
            ))}
          </div>
        ) : null}

        <Button
          size="sm"
          variant="ghost"
          onClick={call.toggleMute}
          data-testid="call-mute"
        >
          {call.micOn ? "Mute" : "Unmute"}
        </Button>
        {call.kind === "video" ? (
          <Button
            size="sm"
            variant="ghost"
            onClick={call.toggleCamera}
            data-testid="call-camera"
          >
            {call.cameraOn ? "Camera off" : "Camera on"}
          </Button>
        ) : null}
        <Button
          size="sm"
          variant="ghost"
          onClick={call.endCall}
          data-testid="call-end"
        >
          End
        </Button>
      </div>
    </div>
  );
}
