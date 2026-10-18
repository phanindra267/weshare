import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CallProvider, useCall } from "@/features/calls/CallProvider";
import { CallOverlay } from "@/features/calls/CallOverlay";
import type { ServerEvent } from "@/features/messaging/types";

type Listener = (event: ServerEvent) => void;

const send = vi.fn();
const subscribe = vi.fn((listener: Listener) => {
  return () => {
    void listener;
  };
});

vi.mock("@/features/messaging/RealtimeProvider", () => ({
  useRealtime: () => ({ send, subscribe, status: "connected", connectionId: 1 }),
}));

vi.mock("@/features/calls/api", () => ({
  callsApi: {
    getIceConfig: vi.fn(async () => ({ ice_servers: [], max_participants: 4 })),
    getHistory: vi.fn(async () => ({ items: [], next_cursor: "" })),
  },
}));

/**
 * Minimal `RTCPeerConnection` stand-in: enough to observe the description
 * exchange, which is what the signalling tests are about.
 */
class FakePeerConnection {
  connectionState = "new";
  localDescription: { type: string; sdp: string } | null = null;
  remoteDescription: { type: string; sdp: string } | null = null;
  onicecandidate: ((event: { candidate: null }) => void) | null = null;
  ontrack: ((event: { track: unknown }) => void) | null = null;
  onconnectionstatechange: (() => void) | null = null;

  private senders: Array<{ track: unknown }> = [];

  addTrack(track: unknown) {
    this.senders.push({ track });
  }

  getSenders() {
    return this.senders;
  }

  async createOffer() {
    return { type: "offer", sdp: "v=0 fake-offer" };
  }

  async createAnswer() {
    return { type: "answer", sdp: "v=0 fake-answer" };
  }

  async setLocalDescription(description: { type: string; sdp: string }) {
    this.localDescription = description;
  }

  async setRemoteDescription(description: { type: string; sdp: string }) {
    this.remoteDescription = description;
  }

  async addIceCandidate() {
    // No ICE in tests; the candidate queue path is exercised implicitly.
  }

  close() {
    this.connectionState = "closed";
  }
}

/** Emits a server event exactly the way the realtime provider would. */
function emit(event: ServerEvent) {
  const listener = subscribe.mock.calls[0]?.[0];
  expect(typeof listener).toBe("function");
  act(() => {
    listener?.(event);
  });
}

function renderOverlay() {
  return render(
    <CallProvider>
      <CallOverlay />
    </CallProvider>,
  );
}

/** Exposes `startCall`, which the overlay itself has no control for. */
function StartButton() {
  const call = useCall();
  if (!call) return null;
  return (
    <button
      type="button"
      onClick={() =>
        void call.startCall({
          kind: "audio",
          participants: [
            { id: "b", display_name: "Bob", username: "bob", avatar_url: null },
          ],
        })
      }
    >
      Call Bob
    </button>
  );
}

describe("CallOverlay", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // jsdom's `navigator` exposes only a getter, so install `mediaDevices`
    // as a configurable own property rather than assigning to `navigator`.
    Object.defineProperty(window.navigator, "mediaDevices", {
      configurable: true,
      value: {
        getUserMedia: vi.fn(async () => ({
          getTracks: () => [],
          getAudioTracks: () => [],
          getVideoTracks: () => [],
        })),
      },
    });
    vi.stubGlobal("RTCPeerConnection", FakePeerConnection);
    // jsdom ships no MediaStream; peer connections create one per participant.
    vi.stubGlobal(
      "MediaStream",
      class FakeMediaStream {
        private tracks: unknown[] = [];
        getTracks() {
          return this.tracks;
        }
        getAudioTracks() {
          return this.tracks;
        }
        getVideoTracks() {
          return this.tracks;
        }
        addTrack(track: unknown) {
          this.tracks.push(track);
        }
        removeTrack(track: unknown) {
          this.tracks = this.tracks.filter((t) => t !== track);
        }
      },
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows the incoming call prompt and accepts", async () => {
    renderOverlay();

    emit({
      type: "call.incoming",
      call_id: "c1",
      from: { id: "a", display_name: "Alice", username: "a", avatar_url: null },
      kind: "video",
      group: false,
      participants: [],
    });

    expect(screen.getByText(/Alice is calling/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /accept/i }));

    await waitFor(() =>
      expect(send).toHaveBeenCalledWith(
        expect.objectContaining({ type: "call.accept", call_id: "c1" }),
      ),
    );
  });

  it("rejects the incoming call", async () => {
    renderOverlay();

    emit({
      type: "call.incoming",
      call_id: "c2",
      from: { id: "b", display_name: "Bob", username: null, avatar_url: null },
      kind: "audio",
      group: false,
      participants: [],
    });

    fireEvent.click(screen.getByRole("button", { name: /reject/i }));

    await waitFor(() =>
      expect(send).toHaveBeenCalledWith(
        expect.objectContaining({ type: "call.reject", call_id: "c2" }),
      ),
    );
    expect(screen.queryByText(/is calling/i)).not.toBeInTheDocument();
  });

  it("stays idle until an invitation arrives", () => {
    renderOverlay();
    expect(screen.queryByRole("button", { name: /accept/i })).not.toBeInTheDocument();
  });

  it("offers to the callee once they accept, glare-free", async () => {
    render(
      <CallProvider>
        <StartButton />
        <CallOverlay />
      </CallProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /call bob/i }));

    await waitFor(() =>
      expect(send).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "call.invite",
          callee_ids: ["b"],
          kind: "audio",
        }),
      ),
    );
    expect(screen.getByText(/in call with bob/i)).toBeInTheDocument();

    emit({ type: "call.accepted", call_id: "c9", user_id: "b" });

    await waitFor(() => {
      const offers = send.mock.calls
        .map((call) => call[0] as Record<string, unknown>)
        .filter((event) => event.type === "call.sdp");
      expect(offers).toHaveLength(1);
      expect(offers[0]?.target_id).toBe("b");
      expect((offers[0]?.sdp as { type: string }).type).toBe("offer");
    });
  });

  it("answers an offer, and never answers an answer", async () => {
    renderOverlay();

    emit({
      type: "call.incoming",
      call_id: "c3",
      from: { id: "a", display_name: "Alice", username: null, avatar_url: null },
      kind: "audio",
      group: false,
      participants: [],
    });
    fireEvent.click(screen.getByRole("button", { name: /accept/i }));
    await waitFor(() =>
      expect(send).toHaveBeenCalledWith(
        expect.objectContaining({ type: "call.accept" }),
      ),
    );

    const sdpSends = () =>
      send.mock.calls
        .map((call) => call[0] as Record<string, unknown>)
        .filter((event) => event.type === "call.sdp");

    emit({
      type: "call.sdp",
      call_id: "c3",
      from_user_id: "a",
      sdp: { type: "offer", sdp: "v=0 remote-offer" },
    });
    await waitFor(() => expect(sdpSends()).toHaveLength(1));
    expect((sdpSends()[0]?.sdp as { type: string }).type).toBe("answer");

    emit({
      type: "call.sdp",
      call_id: "c3",
      from_user_id: "a",
      sdp: { type: "answer", sdp: "v=0 remote-answer" },
    });
    // Applying an answer must not trigger a second answer.
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(sdpSends()).toHaveLength(1);
  });

  it("tears the call down when the server ends it", async () => {
    renderOverlay();

    emit({
      type: "call.incoming",
      call_id: "c4",
      from: { id: "a", display_name: "Alice", username: null, avatar_url: null },
      kind: "video",
      group: false,
      participants: [],
    });
    expect(screen.getByText(/Alice is calling/i)).toBeInTheDocument();

    emit({
      type: "call.ended",
      call_id: "c4",
      reason: "hangup",
      user_id: "a",
    });

    expect(screen.queryByText(/Alice is calling/i)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /end/i })).not.toBeInTheDocument();
  });
});
