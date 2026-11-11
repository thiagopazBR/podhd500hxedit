// A dropdown menu anchored to its button (HX Edit style): one open at a
// time, closes on an outside click or Escape.
import { useEffect, useRef, useState, type ReactNode } from "react";

let closeOpen: (() => void) | null = null;

export function useMenu() {
  const [open, setOpen] = useState(false);
  const wrap = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = () => setOpen(false);
    if (closeOpen && closeOpen !== close) closeOpen();
    closeOpen = close;
    const onDown = (e: MouseEvent) => { if (!wrap.current?.contains(e.target as Node)) close(); };
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") close(); };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
      if (closeOpen === close) closeOpen = null;
    };
  }, [open]);
  return { open, setOpen, toggle: () => setOpen((o) => !o), wrap };
}

export function MenuWrap({ menu, className = "", children }: { menu: ReturnType<typeof useMenu>; className?: string; children: ReactNode }) {
  return <div className={`menu-wrap ${className}`} ref={menu.wrap}>{children}</div>;
}
