"use client";

import * as React from "react";
import { useRealtime } from "@/features/messaging/RealtimeProvider";
import type { ServerEvent } from "@/features/messaging/types";
import { callsApi } from "@/features/calls/api";
import type { CallKind, CallServerEvent, PeerBrief } from "@/features/calls/types";

export type CallState =
  | "idle"
  | "outgoing-ringing"
  | "incoming-ringing"
  | "connecting"
  | "active"
  | "ended";

export interface Peer {
  id: string;
  displayName: string;
  username: string | null;
  avatarUrl: string | null;
  stream: MediaStream | null;
  connected: boolean;
}

export interface IncomingCall {
  callId: string;
  from: PeerBrief;
  kind: CallKind;
  group: boolean;
  participants: string[];
}

interface CallContextValue {
  state: CallState;
  callId: string | null;
  kind: CallKind | null;
  isGroup: boolean;
  isCaller: boolean;
  localStream: MediaStream | null;
  remotePeers: Peer[];
  /** Everyone invited to the current call, known before any media arrives. */
  participants: PeerBrief[];
  incoming: IncomingCall | null;
  micOn: boolean;
  cameraOn: boolean;
  startCall: (params: {
    kind: CallKind;
    participants: PeerBrief[];
  }) => Promise<void>;
  acceptCall: () => Promise<void>;
  rejectCall: () => void;
  endCall: () => void;
  toggleMute: () => void;
  toggleCamera: () => void;
}

const CallContext = React.createContext<CallContextValue | null>(null);

/**
 * Returns the call state, or `null` when no `CallProvider` is mounted.
 *
 * Call affordances render nothing outside the provider instead of throwing, so
 * screens stay usable (and tests stay honest) when calling support is absent.
 */
export function useCall(): CallContextValue | null {
  return React.useContext(CallContext);
}

/** Narrows the realtime event union down to the calling subset. */
function isCallEvent(event: ServerEvent): event is CallServerEvent {
  return event.type.startsWith("call.");
}

function createId(): string {
  try {
    if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
      return crypto.randomUUID();
    }
  } catch {
    // Fall through to the Math.random fallback below.
  }
  return "call_" + Math.random().toString(36).slice(2);
}

/**
 * Owns one call at a time and drives the WebRTC mesh behind it.
 *
 * Group calls are a full peer-to-peer mesh: this component holds one
 * `RTCPeerConnection` per remote participant. To keep negotiation glare-free
 * the *already joined* peer always creates the offer towards a newly joined
 * one, so a joiner only ever answers. Media never passes through the server —
 * the socket only relays SDP and ICE candidates.
 */
export function CallProvider({ children }: { children: React.ReactNode }) {
  const { send, subscribe } = useRealtime();
  const [state, setState] = React.useState<CallState>("idle");
  const [callId, setCallId] = React.useState<string | null>(null);
  const [kind, setKind] = React.useState<CallKind | null>(null);
  const [isGroup, setGroup] = React.useState(false);
  const [isCaller, setCaller] = React.useState(false);
  const [localStream, setLocalStream] = React.useState<MediaStream | null>(null);
  const [micOn, setMicOn] = React.useState(true);
  const [cameraOn, setCameraOn] = React.useState(true);
  const [incoming, setIncoming] = React.useState<IncomingCall | null>(null);
  const [remotePeers, setRemotePeers] = React.useState<Peer[]>([]);
  const [participants, setParticipants] = React.useState<PeerBrief[]>([]);

  // Kept in refs so socket callbacks never read a stale closure.
  const pcs = React.useRef(new Map<string, RTCPeerConnection>());
  const remoteStreams = React.useRef(new Map<string, MediaStream>());
  const briefs = React.useRef(new Map<string, PeerBrief>());
  const pendingCandidates = React.useRef(
    new Map<string, RTCIceCandidateInit[]>(),
  );
  const callIdRef = React.useRef<string | null>(null);
  const localStreamRef = React.useRef<MediaStream | null>(null);
  const stateRef = React.useRef<CallState>("idle");
  const iceServersRef = React.useRef<RTCIceServer[]>([]);
  const iceLoadedRef = React.useRef(false);
  const sendRef = React.useRef(send);

  React.useEffect(() => {
    callIdRef.current = callId;
  }, [callId]);
  React.useEffect(() => {
    localStreamRef.current = localStream;
  }, [localStream]);
  React.useEffect(() => {
    stateRef.current = state;
  }, [state]);
  React.useEffect(() => {
    sendRef.current = send;
  }, [send]);

  const syncPeers = React.useCallback(() => {
    const next: Peer[] = [];
    for (const [id, pc] of pcs.current) {
      const brief = briefs.current.get(id);
      next.push({
        id,
        displayName: brief?.display_name ?? "Participant",
        username: brief?.username ?? null,
        avatarUrl: brief?.avatar_url ?? null,
        stream: remoteStreams.current.get(id) ?? null,
        connected: pc.connectionState === "connected",
      });
    }
    setRemotePeers(next);
  }, []);

  /** Flips the call to `active` once any peer connection is established. */
  const maybeActivate = React.useCallback(() => {
    for (const pc of pcs.current.values()) {
      if (pc.connectionState === "connected") {
        setState("active");
        return;
      }
    }
  }, []);

  /** Fetches STUN/TURN configuration once, falling back to none on failure. */
  const ensureIceServers = React.useCallback(async () => {
    if (iceLoadedRef.current) return;
    iceLoadedRef.current = true;
    try {
      const cfg = await callsApi.getIceConfig();
      iceServersRef.current = (cfg.ice_servers ?? []).map((s) => ({
        urls: s.urls,
        username: s.username,
        credential: s.credential,
      }));
    } catch {
      iceServersRef.current = [];
    }
  }, []);

  const closePeer = React.useCallback(
    (peerId: string) => {
      const pc = pcs.current.get(peerId);
      if (pc) {
        pc.onicecandidate = null;
        pc.ontrack = null;
        pc.onconnectionstatechange = null;
        try {
          pc.close();
        } catch {
          // Already closed or never started; nothing to do.
        }
        pcs.current.delete(peerId);
      }
      remoteStreams.current.delete(peerId);
      pendingCandidates.current.delete(peerId);
      syncPeers();
    },
    [syncPeers],
  );

  const reset = React.useCallback(() => {
    setState("idle");
    setCallId(null);
    setKind(null);
    setGroup(false);
    setCaller(false);
    setIncoming(null);
    setParticipants([]);
    setMicOn(true);
    setCameraOn(true);
    for (const peerId of Array.from(pcs.current.keys())) {
      closePeer(peerId);
    }
    pcs.current.clear();
    remoteStreams.current.clear();
    pendingCandidates.current.clear();
    briefs.current.clear();
    const stream = localStreamRef.current;
    if (stream) {
      for (const track of stream.getTracks()) {
        track.stop();
      }
    }
    localStreamRef.current = null;
    setLocalStream(null);
    setRemotePeers([]);
  }, [closePeer]);

  /** Creates (or reuses) the peer connection for a participant. */
  const ensurePeer = React.useCallback(
    (peerId: string): RTCPeerConnection | null => {
      const existing = pcs.current.get(peerId);
      if (existing) return existing;
      if (typeof RTCPeerConnection === "undefined") return null;

      const pc = new RTCPeerConnection({
        iceServers: iceServersRef.current,
      });
      pcs.current.set(peerId, pc);
      remoteStreams.current.set(peerId, new MediaStream());

      pc.onicecandidate = (event) => {
        const id = callIdRef.current;
        if (!event.candidate || !id) return;
        sendRef.current({
          type: "call.candidate",
          call_id: id,
          target_id: peerId,
          candidate: event.candidate.toJSON(),
        });
      };
      pc.ontrack = (event) => {
        const stream = remoteStreams.current.get(peerId);
        if (stream && !stream.getTracks().includes(event.track)) {
          stream.addTrack(event.track);
        }
        syncPeers();
        maybeActivate();
      };
      pc.onconnectionstatechange = () => {
        syncPeers();
        maybeActivate();
      };
      return pc;
    },
    [maybeActivate, syncPeers],
  );

  /** Publishes our local tracks to a peer connection. */
  const addLocalTracks = React.useCallback((pc: RTCPeerConnection) => {
    const stream = localStreamRef.current;
    if (!stream) return;
    for (const track of stream.getTracks()) {
      const already = pc
        .getSenders()
        .some((sender) => sender.track === track);
      if (!already) pc.addTrack(track, stream);
    }
  }, []);

  /** The already-joined peer offers towards a newly joined one. */
  const offerTo = React.useCallback(
    async (peerId: string) => {
      const id = callIdRef.current;
      const pc = ensurePeer(peerId);
      if (!id || !pc) return;
      addLocalTracks(pc);
      try {
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);
        sendRef.current({
          type: "call.sdp",
          call_id: id,
          target_id: peerId,
          sdp: pc.localDescription,
        });
      } catch {
        // Negotiation failed; the call simply will not connect.
      }
    },
    [addLocalTracks, ensurePeer],
  );

  /**
   * Applies a remote session description and answers only when it is an offer.
   *
   * Our side may act as either end: the already-joined peer offers, so it will
   * receive an *answer* here and must simply apply it. Re-answering an answer
   * would throw an InvalidStateError and stall the connection.
   */
  const handleSdp = React.useCallback(
    async (peerId: string, sdp: unknown, id: string) => {
      const pc = ensurePeer(peerId);
      if (!pc) return;
      addLocalTracks(pc);
      try {
        const description = sdp as RTCSessionDescriptionInit;
        await pc.setRemoteDescription(description);
        for (const candidate of pendingCandidates.current.get(peerId) ?? []) {
          try {
            await pc.addIceCandidate(candidate);
          } catch {
            // A candidate that arrived before the description; skip it.
          }
        }
        pendingCandidates.current.set(peerId, []);
        if (description.type === "answer") return;
        const answer = await pc.createAnswer();
        await pc.setLocalDescription(answer);
        sendRef.current({
          type: "call.sdp",
          call_id: id,
          target_id: peerId,
          sdp: pc.localDescription,
        });
      } catch {
        // Negotiation failed; the call simply will not connect.
      }
    },
    [addLocalTracks, ensurePeer],
  );

  /** Queues a candidate that arrived before its session description. */
  const acceptCandidate = React.useCallback(
    async (peerId: string, candidate: unknown) => {
      const init = candidate as RTCIceCandidateInit;
      const pc = pcs.current.get(peerId);
      if (!pc || !pc.remoteDescription) {
        const queue = pendingCandidates.current.get(peerId) ?? [];
        queue.push(init);
        pendingCandidates.current.set(peerId, queue);
        return;
      }
      try {
        await pc.addIceCandidate(init);
      } catch {
        // Unusable candidate; the connection can still form from the others.
      }
    },
    [],
  );

  const getUserMedia = React.useCallback(async (k: CallKind) => {
    const constraints: MediaStreamConstraints = {
      audio: true,
      video:
        k === "video"
          ? { width: { ideal: 1280 }, height: { ideal: 720 } }
          : false,
    };
    return navigator.mediaDevices.getUserMedia(constraints);
  }, []);

  React.useEffect(() => {
    const unsubscribe = subscribe((raw) => {
      if (!isCallEvent(raw)) return;
      const event = raw;
      switch (event.type) {
        case "call.incoming": {
          if (stateRef.current !== "idle") {
            sendRef.current({ type: "call.busy", call_id: event.call_id });
            return;
          }
          briefs.current.set(event.from.id, event.from);
          setIncoming({
            callId: event.call_id,
            from: event.from,
            kind: event.kind,
            group: event.group,
            participants: event.participants,
          });
          callIdRef.current = event.call_id;
          setCallId(event.call_id);
          setKind(event.kind);
          setGroup(event.group);
          setCaller(false);
          setParticipants([event.from]);
          setState("incoming-ringing");
          return;
        }
        case "call.accepted": {
          const known = briefs.current.get(event.user_id);
          const brief: PeerBrief = {
            id: event.user_id,
            username: known?.username ?? null,
            display_name: known?.display_name ?? "Participant",
            avatar_url: known?.avatar_url ?? null,
          };
          briefs.current.set(event.user_id, brief);
          setParticipants((prev) =>
            prev.some((p) => p.id === brief.id) ? prev : [...prev, brief],
          );
          if (stateRef.current === "outgoing-ringing") {
            setState("connecting");
          }
          // Glare-free: the peer who was already in the call makes the offer.
          void offerTo(event.user_id);
          syncPeers();
          return;
        }
        case "call.sdp": {
          void handleSdp(event.from_user_id, event.sdp, event.call_id);
          return;
        }
        case "call.candidate": {
          void acceptCandidate(event.from_user_id, event.candidate);
          return;
        }
        case "call.participants": {
          for (const brief of event.participants) {
            briefs.current.set(brief.id, brief);
          }
          setParticipants((prev) => {
            const next = [...prev];
            for (const brief of event.participants) {
              const index = next.findIndex((p) => p.id === brief.id);
              if (index === -1) next.push(brief);
              else next[index] = brief;
            }
            return next;
          });
          syncPeers();
          return;
        }
        case "call.rejected": {
          // A direct call also receives `call.ended` right after, but a group
          // call keeps running, so drop only the participant who declined.
          briefs.current.delete(event.user_id);
          closePeer(event.user_id);
          setParticipants((prev) =>
            prev.filter((p) => p.id !== event.user_id),
          );
          return;
        }
        case "call.ended": {
          if (event.call_id !== callIdRef.current) return;
          // In a group, `left` only tears down that participant's connection.
          if (
            event.reason === "left" &&
            event.user_id &&
            pcs.current.has(event.user_id)
          ) {
            closePeer(event.user_id);
            return;
          }
          reset();
          return;
        }
        default:
          return;
      }
    });
    return unsubscribe;
  }, [acceptCandidate, closePeer, handleSdp, offerTo, reset, subscribe, syncPeers]);

  const startCall = React.useCallback(
    async ({
      kind: requested,
      participants,
    }: {
      kind: CallKind;
      participants: PeerBrief[];
    }) => {
      if (stateRef.current !== "idle" || participants.length === 0) return;
      await ensureIceServers();
      let stream: MediaStream;
      try {
        stream = await getUserMedia(requested);
      } catch {
        return;
      }
      const id = createId();
      localStreamRef.current = stream;
      setLocalStream(stream);
      setMicOn(true);
      setCameraOn(requested === "video");
      setKind(requested);
      setGroup(participants.length > 1);
      setCaller(true);
      for (const brief of participants) briefs.current.set(brief.id, brief);
      setParticipants(participants);
      callIdRef.current = id;
      setCallId(id);
      stateRef.current = "outgoing-ringing";
      setState("outgoing-ringing");
      sendRef.current({
        type: "call.invite",
        call_id: id,
        callee_ids: participants.map((p) => p.id),
        kind: requested,
      });
    },
    [ensureIceServers, getUserMedia],
  );

  const acceptCall = React.useCallback(async () => {
    const current = incoming;
    if (!current || stateRef.current !== "incoming-ringing") return;
    await ensureIceServers();
    let stream: MediaStream;
    try {
      stream = await getUserMedia(current.kind);
    } catch {
      sendRef.current({ type: "call.reject", call_id: current.callId });
      reset();
      return;
    }
    localStreamRef.current = stream;
    setLocalStream(stream);
    setMicOn(true);
    setCameraOn(current.kind === "video");
    stateRef.current = "connecting";
    setState("connecting");
    sendRef.current({ type: "call.accept", call_id: current.callId });
  }, [ensureIceServers, getUserMedia, incoming, reset]);

  const rejectCall = React.useCallback(() => {
    const current = incoming;
    if (!current || stateRef.current !== "incoming-ringing") return;
    sendRef.current({ type: "call.reject", call_id: current.callId });
    reset();
  }, [incoming, reset]);

  const endCall = React.useCallback(() => {
    if (!callIdRef.current) return;
    sendRef.current({ type: "call.end", call_id: callIdRef.current });
    reset();
  }, [reset]);

  const toggleMute = React.useCallback(() => {
    setMicOn((on) => {
      for (const track of localStreamRef.current?.getAudioTracks() ?? []) {
        track.enabled = !on;
      }
      return !on;
    });
  }, []);

  const toggleCamera = React.useCallback(() => {
    setCameraOn((on) => {
      for (const track of localStreamRef.current?.getVideoTracks() ?? []) {
        track.enabled = !on;
      }
      return !on;
    });
  }, []);

  // Tear down media when the provider unmounts.
  React.useEffect(() => reset, [reset]);

  const value: CallContextValue = {
    state,
    callId,
    kind,
    isGroup,
    isCaller,
    localStream,
    remotePeers,
    participants,
    incoming,
    micOn,
    cameraOn,
    startCall,
    acceptCall,
    rejectCall,
    endCall,
    toggleMute,
    toggleCamera,
  };

  return <CallContext.Provider value={value}>{children}</CallContext.Provider>;
}
