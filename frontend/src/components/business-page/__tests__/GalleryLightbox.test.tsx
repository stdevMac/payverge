/** @jest-environment jsdom */
import { render, screen, fireEvent } from "@testing-library/react";
import GalleryLightbox from "../GalleryLightbox";

const t = (k: string) => k;

const images = [
  { id: 1, image_url: "https://example.com/a.jpg", caption: "Terrace at sunset" },
  { id: 2, image_url: "https://example.com/b.jpg", caption: "" },
  { id: 3, image_url: "https://example.com/c.jpg", caption: "Chef's counter" },
];

describe("GalleryLightbox (plan 1.6)", () => {
  it("renders a labelled modal dialog with the current image and caption", () => {
    render(
      <GalleryLightbox images={images} initialIndex={0} t={t} onClose={jest.fn()} />,
    );
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(screen.getByText("Terrace at sunset")).toBeInTheDocument();
    expect(screen.getByText("1 / 3")).toBeInTheDocument();
  });

  it("closes on Escape", () => {
    const onClose = jest.fn();
    render(
      <GalleryLightbox images={images} initialIndex={0} t={t} onClose={onClose} />,
    );
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("closes when the backdrop (outside the panel) is pressed", () => {
    const onClose = jest.fn();
    render(
      <GalleryLightbox images={images} initialIndex={0} t={t} onClose={onClose} />,
    );
    // The presentation wrapper is the outside-click target.
    fireEvent.mouseDown(
      document.querySelector(".storefront-gallery-lightbox") as HTMLElement,
    );
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does NOT close when the panel itself is pressed", () => {
    const onClose = jest.fn();
    render(
      <GalleryLightbox images={images} initialIndex={0} t={t} onClose={onClose} />,
    );
    fireEvent.mouseDown(screen.getByRole("dialog"));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("closes via the close button", () => {
    const onClose = jest.fn();
    render(
      <GalleryLightbox images={images} initialIndex={0} t={t} onClose={onClose} />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "businessPage.gallery.closeLightbox" }),
    );
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("navigates with next/previous buttons and wraps around", () => {
    render(
      <GalleryLightbox images={images} initialIndex={0} t={t} onClose={jest.fn()} />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "businessPage.gallery.nextImage" }),
    );
    expect(screen.getByText("2 / 3")).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "businessPage.gallery.previousImage" }),
    );
    expect(screen.getByText("1 / 3")).toBeInTheDocument();
    // Wrap backwards past the first image → last.
    fireEvent.click(
      screen.getByRole("button", { name: "businessPage.gallery.previousImage" }),
    );
    expect(screen.getByText("3 / 3")).toBeInTheDocument();
    expect(screen.getByText("Chef's counter")).toBeInTheDocument();
  });

  it("navigates with ArrowRight / ArrowLeft keys", () => {
    render(
      <GalleryLightbox images={images} initialIndex={0} t={t} onClose={jest.fn()} />,
    );
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "ArrowRight" });
    expect(screen.getByText("2 / 3")).toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "ArrowLeft" });
    expect(screen.getByText("1 / 3")).toBeInTheDocument();
  });

  it("hides arrows and the counter for a single image", () => {
    render(
      <GalleryLightbox
        images={[images[0]]}
        initialIndex={0}
        t={t}
        onClose={jest.fn()}
      />,
    );
    expect(
      screen.queryByRole("button", { name: "businessPage.gallery.nextImage" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("1 / 1")).not.toBeInTheDocument();
  });

  it("locks body scroll while open and restores it on unmount", () => {
    const { unmount } = render(
      <GalleryLightbox images={images} initialIndex={0} t={t} onClose={jest.fn()} />,
    );
    expect(document.body.style.overflow).toBe("hidden");
    unmount();
    expect(document.body.style.overflow).toBe("");
  });

  it("shows a labelled fallback tile instead of a broken image", () => {
    render(
      <GalleryLightbox images={images} initialIndex={0} t={t} onClose={jest.fn()} />,
    );
    const img = screen.getByRole("img", { name: "Terrace at sunset" });
    fireEvent.error(img);
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    // Caption text remains as the fallback tile content (and as the caption).
    expect(screen.getAllByText("Terrace at sunset").length).toBeGreaterThan(0);
  });
});
