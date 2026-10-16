import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "@/components/ui/Toast";

const { listMock, listRequestsMock, acceptMock, sendRequestMock } = vi.hoisted(
  () => ({
    listMock: vi.fn(),
    listRequestsMock: vi.fn(),
    acceptMock: vi.fn(),
    sendRequestMock: vi.fn(),
  }),
);

vi.mock("@/features/contacts/api", () => ({
  contactsApi: {
    list: listMock,
    listRequests: listRequestsMock,
    accept: acceptMock,
    decline: vi.fn(),
    remove: vi.fn(),
    block: vi.fn(),
    unblock: vi.fn(),
    sendRequest: sendRequestMock,
  },
}));

import { ContactsView } from "@/features/contacts/ContactsView";

const alice = {
  id: "11111111-1111-1111-1111-111111111111",
  username: "alice",
  display_name: "Alice",
  avatar_url: null,
  status_message: "hi",
  online: true,
  last_seen_at: null,
};

const bob = {
  id: "22222222-2222-2222-2222-222222222222",
  username: "bob",
  display_name: "Bob",
  avatar_url: null,
  status_message: null,
  online: false,
  last_seen_at: null,
};

function renderView() {
  return render(
    <ToastProvider>
      <ContactsView />
    </ToastProvider>,
  );
}

describe("ContactsView", () => {
  beforeEach(() => {
    listMock.mockReset();
    listRequestsMock.mockReset();
    acceptMock.mockReset();
    sendRequestMock.mockReset();

    listMock.mockResolvedValue({
      items: [{ user: alice, connected_at: "2026-01-01T00:00:00Z" }],
      next_cursor: "",
    });
    listRequestsMock.mockImplementation((direction: string) =>
      Promise.resolve({
        items:
          direction === "incoming"
            ? [
                {
                  id: "req-1",
                  status: "pending",
                  direction: "incoming",
                  user: bob,
                  created_at: "2026-01-01T00:00:00Z",
                },
              ]
            : [],
        next_cursor: "",
      }),
    );
  });

  it("lists contacts and incoming requests", async () => {
    renderView();
    expect(await screen.findByText("Alice")).toBeInTheDocument();
    expect(await screen.findByText("Bob")).toBeInTheDocument();
  });

  it("accepts an incoming request", async () => {
    acceptMock.mockResolvedValue({});
    renderView();

    await screen.findByText("Bob");
    await userEvent.click(screen.getByRole("button", { name: /accept/i }));

    await waitFor(() => {
      expect(acceptMock).toHaveBeenCalledWith("req-1");
    });
  });

  it("sends a friend request by user id", async () => {
    sendRequestMock.mockResolvedValue({ status: "pending" });
    renderView();

    await screen.findByText("Alice");
    await userEvent.type(
      screen.getByLabelText(/user id/i),
      "33333333-3333-3333-3333-333333333333",
    );
    await userEvent.click(
      screen.getByRole("button", { name: /send request/i }),
    );

    await waitFor(() => {
      expect(sendRequestMock).toHaveBeenCalledWith(
        "33333333-3333-3333-3333-333333333333",
      );
    });
  });
});
