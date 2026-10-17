import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { UploadProgress } from "./UploadProgress";

describe("UploadProgress", () => {
  it("renders progressbar with appropriate accessibility attributes", () => {
    render(<UploadProgress percent={45} />);
    const bar = screen.getByRole("progressbar");
    expect(bar).toBeInTheDocument();
    expect(bar).toHaveAttribute("aria-valuenow", "45");
    expect(bar).toHaveAttribute("aria-valuemin", "0");
    expect(bar).toHaveAttribute("aria-valuemax", "100");
    expect(bar).toHaveAttribute("aria-label", "Uploading: 45%");
  });

  it("clamps percent to valid range", () => {
    const { container } = render(<UploadProgress percent={120} />);
    const inner = container.querySelector(".bg-brand-600");
    expect(inner).toHaveStyle({ width: "100%" });
  });
});
